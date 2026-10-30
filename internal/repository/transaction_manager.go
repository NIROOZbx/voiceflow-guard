package repository

import (
	"context"

	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Transaction struct {
	db *pgxpool.Pool
}

type Repositories struct {
	Operations OperationsRepository
	Bookings   BookingsRepository
}

type TransactionManager interface {
	WithinTransaction(ctx context.Context, fn func(*Repositories) error)error
}

func NewTransactionManager(db *pgxpool.Pool) *Transaction {
	return &Transaction{db: db}
}

func (t *Transaction) WithinTransaction(ctx context.Context, fn func(*Repositories) error) error {

	tx, err := t.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer tx.Rollback(ctx)
	queries := sqlc.New(tx)

	repos := &Repositories{
		Operations: NewOpsRepo(queries),
		Bookings:   NewBookingsRepo(queries),
	}
	
	if err := fn(repos); err != nil {
    return err
}

return tx.Commit(ctx)

}
