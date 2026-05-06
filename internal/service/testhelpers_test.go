//go:build !integration

// Package service_test contains shared test helpers for the service layer.
// This file is excluded when running with -tags integration.
// unitFakeTxBeginner is defined in testhelpers_shared_test.go (no build tag).
package service_test

// newIntegrationFakeTxBeginner returns the standard unit-test fake transaction
// beginner. When running without the integration tag, this function is the
// primary entry-point used by auth_test.go, supplier_test.go, etc.
func newIntegrationFakeTxBeginner() *unitFakeTxBeginner {
	return &unitFakeTxBeginner{}
}
