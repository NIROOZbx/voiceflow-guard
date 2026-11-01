package repository

import (
	"context"

	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

type OperationsRepo struct {
	db *sqlc.Queries
}

type OperationsRepository interface {
	CreateOperation(ctx context.Context, key string, requestHash string) (sqlc.Operation, error)
	GetOperationByIdempotencyKey(ctx context.Context, key string) (sqlc.Operation, error)
	UpdateOperationStatus(ctx context.Context, id pgtype.UUID, status string, result []byte) (sqlc.Operation, error)
}

func NewOpsRepo(db *sqlc.Queries) *OperationsRepo {
	return &OperationsRepo{
		db: db,
	}
}

func (o *OperationsRepo) CreateOperation(ctx context.Context, key, requestHash string) (sqlc.Operation, error) {
	operation, err := o.db.CreateOperation(ctx, sqlc.CreateOperationParams{
		IdempotencyKey: key,
		RequestHash: pgtype.Text{
			String: requestHash,
			Valid:  true,
		},
	})
	if err != nil {
		return sqlc.Operation{}, err
	}

	return operation, nil
}

func (o *OperationsRepo) GetOperationByIdempotencyKey(ctx context.Context, key string) (sqlc.Operation, error) {
	operation, err := o.db.GetOperationByIdempotencyKey(ctx, key)
	if err != nil {
		return sqlc.Operation{}, err
	}

	return operation, nil
}

func (o *OperationsRepo) UpdateOperationStatus(ctx context.Context, id pgtype.UUID, status string, result []byte) (sqlc.Operation, error) {

	operation, err := o.db.UpdateOperationStatus(ctx, sqlc.UpdateOperationStatusParams{
		ID:     id,
		Status: status,
		Result: result,
	})
	if err != nil {
		return sqlc.Operation{}, err
	}

	return operation, nil
}
