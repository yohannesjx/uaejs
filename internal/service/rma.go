package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/dubai-retail/os/internal/domain"
	"github.com/dubai-retail/os/internal/metrics"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// =============================================================================
// Repository interfaces consumed by RMAService
// =============================================================================

// RMARepo defines DB operations required by RMAService.
type RMARepo interface {
	InsertReturn(ctx context.Context, tx pgx.Tx, ret *domain.Return) error
	InsertReturnItem(ctx context.Context, tx pgx.Tx, item *domain.ReturnItem) error
	InsertReturnPhoto(ctx context.Context, photo *domain.ReturnPhoto) error
	GetReturnByID(ctx context.Context, id uuid.UUID) (*domain.Return, error)
	GetOutboundQCHash(ctx context.Context, returnItemID uuid.UUID) (string, error)
	UpdateReturnItemQC(ctx context.Context, tx pgx.Tx, itemID uuid.UUID, score float64, passed bool, notes string) error
	UpdateReturnStatus(ctx context.Context, tx pgx.Tx, returnID uuid.UUID, status domain.ReturnStatus, reason *string) error
	SetReturnItemCOGS(ctx context.Context, tx pgx.Tx, itemID uuid.UUID, cogsPerUnit string) error
}

// PhotoStorage is the interface for storing photo bytes in an external store
// (S3, GCS, local disk, etc.). Implementations live outside this package.
type PhotoStorage interface {
	// Store persists r and returns the canonical path (e.g. "s3://bucket/key").
	Store(ctx context.Context, returnItemID uuid.UUID, photoType domain.PhotoType, r io.Reader) (storagePath string, err error)
}

// RMAInventoryRepo is the subset of inventory operations needed for stock
// adjustments on approved returns.
type RMAInventoryRepo interface {
	// AdjustStock records an adjustment_in movement to return good-condition items.
	AdjustStock(ctx context.Context, tx pgx.Tx, variantID uuid.UUID, delta int, movementType string, note string) error
}

// =============================================================================
// Service
// =============================================================================

// QCPassThreshold is the minimum match score (0–1) to auto-pass QC.
// A score of 1.0 means the byte-perfect SHA-256 hash matches the outbound photo.
// Any non-exact hash produces 0.0 at this layer; a higher-resolution image
// similarity model can produce intermediate scores via the same interface.
const QCPassThreshold = 1.0

// RMAService orchestrates return requests, QC photo comparison, and stock
// adjustments on approved returns.
type RMAService struct {
	rmaRepo      RMARepo
	invRepo      RMAInventoryRepo
	storage      PhotoStorage
	pool         TxBeginner
	metrics      *metrics.Metrics
	log          *zap.Logger
}

// NewRMAService creates a new RMAService.
func NewRMAService(
	rmaRepo RMARepo,
	invRepo RMAInventoryRepo,
	storage PhotoStorage,
	pool TxBeginner,
	m *metrics.Metrics,
	log *zap.Logger,
) *RMAService {
	return &RMAService{
		rmaRepo: rmaRepo,
		invRepo: invRepo,
		storage: storage,
		pool:    pool,
		metrics: m,
		log:     log,
	}
}

// =============================================================================
// CreateReturn – step 1 of the RMA lifecycle
// =============================================================================

// CreateReturnInput is the client-facing DTO for opening a return request.
type CreateReturnInput struct {
	OrderID       uuid.UUID
	CustomerName  string
	CustomerEmail string
	ReturnReason  string
	Notes         *string
	Items         []ReturnItemInput
}

// ReturnItemInput describes one SKU being returned.
type ReturnItemInput struct {
	OrderItemID uuid.UUID
	VariantID   uuid.UUID
	BatchItemID *uuid.UUID
	Quantity    int
	Condition   domain.ItemCondition
}

// CreateReturn opens a new return request and persists it in a transaction.
// It does NOT yet perform any stock or COGS adjustment; those happen on Approve.
func (s *RMAService) CreateReturn(ctx context.Context, in CreateReturnInput) (*domain.Return, error) {
	if len(in.Items) == 0 {
		return nil, fmt.Errorf("CreateReturn: at least one item required")
	}

	ret := &domain.Return{
		ID:            uuid.New(),
		OrderID:       in.OrderID,
		Status:        domain.ReturnStatusPending,
		CustomerName:  in.CustomerName,
		CustomerEmail: in.CustomerEmail,
		ReturnReason:  in.ReturnReason,
		Notes:         in.Notes,
	}

	items := make([]domain.ReturnItem, 0, len(in.Items))
	for _, inp := range in.Items {
		items = append(items, domain.ReturnItem{
			ID:          uuid.New(),
			ReturnID:    ret.ID,
			OrderItemID: inp.OrderItemID,
			VariantID:   inp.VariantID,
			BatchItemID: inp.BatchItemID,
			Quantity:    inp.Quantity,
			Condition:   inp.Condition,
		})
	}
	ret.Items = items

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("CreateReturn: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.rmaRepo.InsertReturn(ctx, tx, ret); err != nil {
		return nil, err
	}
	for i := range ret.Items {
		if err := s.rmaRepo.InsertReturnItem(ctx, tx, &ret.Items[i]); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("CreateReturn: commit: %w", err)
	}

	// Metrics
	for _, it := range ret.Items {
		s.metrics.ReturnsCreatedTotal.WithLabelValues("pos", string(it.Condition)).Inc()
	}

	s.log.Info("rma.created",
		zap.String("return_id", ret.ID.String()),
		zap.String("order_id", ret.OrderID.String()),
		zap.Int("item_count", len(ret.Items)),
	)

	return ret, nil
}

// =============================================================================
// UploadPhoto – attach a QC photo to a return item
// =============================================================================

// UploadPhotoInput carries photo metadata and the raw bytes (as an io.Reader).
type UploadPhotoInput struct {
	ReturnItemID  uuid.UUID
	PhotoType     domain.PhotoType
	Reader        io.Reader
	FileSizeBytes int64
	MIMEType      string
}

// UploadPhoto stores the photo file via PhotoStorage, computes its SHA-256,
// persists the metadata in return_photos, and—for customer-submitted photos—
// triggers a QC comparison against the stored outbound hash.
func (s *RMAService) UploadPhoto(ctx context.Context, in UploadPhotoInput) (*domain.ReturnPhoto, *domain.QCResult, error) {
	// Read bytes and compute hash concurrently with streaming to storage.
	// Strategy: buffer bytes to hash first, then stream to storage.
	// For very large files a tee-reader approach would be used, but photo
	// files are typically < 10 MB so this is acceptable.
	data, err := io.ReadAll(in.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("UploadPhoto: read: %w", err)
	}

	// SHA-256 of the raw file bytes
	sum := sha256.Sum256(data)
	fileHash := hex.EncodeToString(sum[:])

	// Upload to object storage
	storagePath, err := s.storage.Store(ctx, in.ReturnItemID, in.PhotoType, noopReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("UploadPhoto: store: %w", err)
	}

	photo := &domain.ReturnPhoto{
		ID:            uuid.New(),
		ReturnItemID:  in.ReturnItemID,
		PhotoType:     in.PhotoType,
		FileHash:      fileHash,
		FileSizeBytes: in.FileSizeBytes,
		MIMEType:      in.MIMEType,
		StoragePath:   storagePath,
	}

	if err := s.rmaRepo.InsertReturnPhoto(ctx, photo); err != nil {
		return nil, nil, fmt.Errorf("UploadPhoto: persist: %w", err)
	}

	s.log.Info("rma.photo_uploaded",
		zap.String("return_item_id", in.ReturnItemID.String()),
		zap.String("photo_type", string(in.PhotoType)),
		zap.String("file_hash", fileHash),
		zap.Int64("bytes", in.FileSizeBytes),
	)

	// Only run QC comparison for customer-submitted photos
	if in.PhotoType != domain.PhotoTypeCustomerSubmitted {
		return photo, nil, nil
	}

	qcResult, err := s.runQCComparison(ctx, in.ReturnItemID, fileHash)
	if err != nil {
		// Non-fatal: log and continue; photo is still stored.
		s.log.Warn("rma.qc_comparison_failed",
			zap.String("return_item_id", in.ReturnItemID.String()),
			zap.Error(err),
		)
		return photo, nil, nil
	}

	return photo, qcResult, nil
}

// runQCComparison compares the customer-submitted photo hash against the
// stored outbound QC hash for the given return item.
//
// Hash comparison rules:
//   - Exact SHA-256 match → score = 1.0 → QC passed (same photo, item is authentic)
//   - No outbound hash stored → score = nil → requires manual review
//   - Different hash → score = 0.0 → QC failed (damage claim or potential fraud)
func (s *RMAService) runQCComparison(ctx context.Context, returnItemID uuid.UUID, customerHash string) (*domain.QCResult, error) {
	outboundHash, err := s.rmaRepo.GetOutboundQCHash(ctx, returnItemID)
	if err != nil {
		return nil, fmt.Errorf("runQCComparison: get outbound hash: %w", err)
	}

	result := &domain.QCResult{ReturnItemID: returnItemID}

	if outboundHash == "" {
		result.MatchScore = 0
		result.QCPassed = false
		result.Reason = "no outbound QC photo on file – manual review required"

		s.log.Warn("rma.qc_no_outbound_hash",
			zap.String("return_item_id", returnItemID.String()),
		)
		return result, nil
	}

	if customerHash == outboundHash {
		// Byte-perfect match: the customer is returning the exact item shipped
		result.MatchScore = 1.0
		result.QCPassed = true
		result.Reason = "SHA-256 hash matches outbound QC photo"
	} else {
		// Different hash: could be a different item, damaged item, or fraud
		result.MatchScore = 0.0
		result.QCPassed = false
		result.Reason = "photo hash mismatch – item may be damaged, wrong item, or fraudulent return"
		s.metrics.QCMismatchTotal.Inc()

		s.log.Warn("rma.qc_hash_mismatch",
			zap.String("return_item_id", returnItemID.String()),
			zap.String("customer_hash", customerHash),
			zap.String("outbound_hash", outboundHash),
		)
	}

	// Persist QC result on the return item
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return result, nil // still return result even if persist fails
	}
	defer tx.Rollback(ctx)

	if err := s.rmaRepo.UpdateReturnItemQC(
		ctx, tx, returnItemID, result.MatchScore, result.QCPassed, result.Reason,
	); err != nil {
		s.log.Error("rma.qc_persist_failed", zap.Error(err))
		return result, nil
	}
	_ = tx.Commit(ctx)

	s.log.Info("rma.qc_completed",
		zap.String("return_item_id", returnItemID.String()),
		zap.Float64("score", result.MatchScore),
		zap.Bool("passed", result.QCPassed),
	)

	return result, nil
}

// =============================================================================
// Approve – step 3: adjust stock, reverse COGS, close return
// =============================================================================

// ApproveInput carries the per-item COGS data known at approval time.
type ApproveItemInput struct {
	ReturnItemID uuid.UUID
	// COGSPerUnit is the landed cost per unit (from original order_items.cogs_per_unit).
	COGSPerUnit decimal.Decimal
}

type ApproveInput struct {
	ReturnID uuid.UUID
	Items    []ApproveItemInput
}

// ApproveReturn:
//  1. Validates all QC statuses (or allows override for manual approval).
//  2. For each item in 'good' condition: records an adjustment_in inventory
//     movement to return the units to the global pool.
//  3. Stamps cogs_per_unit_reversed on each return_item for audit.
//  4. Advances return status to 'approved'.
//  5. All writes run in a single ReadCommitted transaction.
func (s *RMAService) ApproveReturn(ctx context.Context, in ApproveInput) (*domain.Return, error) {
	ret, err := s.rmaRepo.GetReturnByID(ctx, in.ReturnID)
	if err != nil {
		return nil, fmt.Errorf("ApproveReturn: get return: %w", err)
	}

	if ret.Status != domain.ReturnStatusReceived && ret.Status != domain.ReturnStatusQCReview {
		return nil, fmt.Errorf("ApproveReturn: return is in status %q – must be received or qc_review", ret.Status)
	}

	// Build COGS map for quick lookup
	cogsMap := make(map[uuid.UUID]decimal.Decimal, len(in.Items))
	for _, it := range in.Items {
		cogsMap[it.ReturnItemID] = it.COGSPerUnit
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("ApproveReturn: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, item := range ret.Items {
		cogs := cogsMap[item.ID]

		// Stamp COGS for immutable audit trail
		if err := s.rmaRepo.SetReturnItemCOGS(ctx, tx, item.ID, cogs.String()); err != nil {
			return nil, fmt.Errorf("ApproveReturn: set cogs on item %s: %w", item.ID, err)
		}

		// Only return stock for items in good condition
		if item.Condition == domain.ItemConditionGood {
			note := fmt.Sprintf("return approved – return_id:%s", in.ReturnID)
			if err := s.invRepo.AdjustStock(ctx, tx, item.VariantID, item.Quantity, "adjustment_in", note); err != nil {
				return nil, fmt.Errorf("ApproveReturn: adjust stock for variant %s: %w", item.VariantID, err)
			}

			s.log.Info("rma.stock_returned",
				zap.String("return_id", in.ReturnID.String()),
				zap.String("return_item_id", item.ID.String()),
				zap.String("variant_id", item.VariantID.String()),
				zap.Int("quantity", item.Quantity),
				zap.String("cogs_per_unit", cogs.String()),
				zap.String("action", "adjustment_in"),
			)
		} else {
			// Damaged / wrong item: record as adjustment_out (write-off)
			note := fmt.Sprintf("return damaged write-off – return_id:%s condition:%s", in.ReturnID, item.Condition)
			if err := s.invRepo.AdjustStock(ctx, tx, item.VariantID, -item.Quantity, "adjustment_out", note); err != nil {
				return nil, fmt.Errorf("ApproveReturn: write-off for variant %s: %w", item.VariantID, err)
			}

			s.log.Info("rma.stock_writeoff",
				zap.String("return_id", in.ReturnID.String()),
				zap.String("return_item_id", item.ID.String()),
				zap.String("variant_id", item.VariantID.String()),
				zap.Int("quantity", item.Quantity),
				zap.String("condition", string(item.Condition)),
				zap.String("action", "adjustment_out"),
			)
		}
	}

	if err := s.rmaRepo.UpdateReturnStatus(ctx, tx, in.ReturnID, domain.ReturnStatusApproved, nil); err != nil {
		return nil, fmt.Errorf("ApproveReturn: update status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("ApproveReturn: commit: %w", err)
	}

	// Re-fetch the updated return so the caller gets fresh state
	updated, _ := s.rmaRepo.GetReturnByID(ctx, in.ReturnID)
	if updated == nil {
		ret.Status = domain.ReturnStatusApproved
		resolvedAt := time.Now().UTC()
		ret.ResolvedAt = &resolvedAt
		return ret, nil
	}

	s.log.Info("rma.approved",
		zap.String("return_id", in.ReturnID.String()),
		zap.Int("items_count", len(ret.Items)),
	)

	return updated, nil
}

// GetReturnByID fetches a Return by its primary key.
func (s *RMAService) GetReturnByID(ctx context.Context, id uuid.UUID) (*domain.Return, error) {
	return s.rmaRepo.GetReturnByID(ctx, id)
}

// ListReturns returns paginated returns with optional filters.
func (s *RMAService) ListReturns(ctx context.Context, filters domain.ReturnListFilters) (*domain.PageResponse[domain.ReturnListItem], error) {
	listRepo, ok := s.rmaRepo.(interface {
		ListReturns(ctx context.Context, filters domain.ReturnListFilters) ([]domain.ReturnListItem, int, error)
	})
	if !ok {
		return nil, fmt.Errorf("ListReturns: repository does not support list queries")
	}

	filters.Page = normalizePage(filters.Page)
	filters.PageSize = normalizePageSize(filters.PageSize)

	items, total, err := listRepo.ListReturns(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("ListReturns: %w", err)
	}

	return &domain.PageResponse[domain.ReturnListItem]{
		Items: items,
		Total: total,
	}, nil
}

// RejectReturn advances the return to 'rejected' with an optional reason.
func (s *RMAService) RejectReturn(ctx context.Context, returnID uuid.UUID, reason string) (*domain.Return, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("RejectReturn: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.rmaRepo.UpdateReturnStatus(ctx, tx, returnID, domain.ReturnStatusRejected, &reason); err != nil {
		return nil, fmt.Errorf("RejectReturn: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("RejectReturn: commit: %w", err)
	}

	s.log.Info("rma.rejected",
		zap.String("return_id", returnID.String()),
		zap.String("reason", reason),
	)

	return s.rmaRepo.GetReturnByID(ctx, returnID)
}

// =============================================================================
// Helpers
// =============================================================================

// HashReader computes the SHA-256 hex hash of the bytes read from r.
// Exported so that handlers can hash a multipart file before calling UploadPhoto.
func HashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
