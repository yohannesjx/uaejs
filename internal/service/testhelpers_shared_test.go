// Package service_test shared test helpers — no build tag so they compile in
// both standard and "-tags integration" modes.
package service_test

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// unitFakeTxBeginner is a no-op transaction beginner for unit tests.
// It is shared between the standard test build and the integration test build.
type unitFakeTxBeginner struct{}

func (f *unitFakeTxBeginner) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return &fakeTx{}, nil
}
