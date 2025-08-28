package database

import (
	"time"
	"context"
	"errors"
	
	"github.com/go-worker-ledger/internal/core/model"

	go_core_observ "github.com/eliezerraj/go-core/observability"
	go_core_pg "github.com/eliezerraj/go-core/database/pg"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

var (
	childLogger = log.With().Str("component","go-worker-ledger").Str("package","internal.adapter.database").Logger()
	tracerProvider go_core_observ.TracerProvider
)

type WorkerRepository struct {
	DatabasePGServer *go_core_pg.DatabasePGServer
}

// About NewWorkerRepository
func NewWorkerRepository(databasePGServer *go_core_pg.DatabasePGServer) *WorkerRepository{
	childLogger.Info().Msg("NewWorkerRepository")

	return &WorkerRepository{
		DatabasePGServer: databasePGServer,
	}
}

// About create GetTransactionUUID
func (w WorkerRepository) GetTransactionUUID(ctx context.Context) (*string, error){
	childLogger.Info().Interface("trace-resquest-id", ctx.Value("trace-request-id")).Msg("GetTransactionUUID")
	
	// Trace
	span := tracerProvider.Span(ctx, "database.GetTransactionUUID")
	defer span.End()

	// prepare database connection
	conn, err := w.DatabasePGServer.Acquire(ctx)
	if err != nil {
		return nil, errors.New(err.Error())
	}
	defer w.DatabasePGServer.Release(conn)

	// Prepare
	var uuid string

	// Query and Execute
	query := `SELECT uuid_generate_v4()`

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, errors.New(err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(&uuid) 
		if err != nil {
			return nil, errors.New(err.Error())
        }
		return &uuid, nil
	}
	
	return &uuid, nil
}

// About update pix_transaction
func (w *WorkerRepository) UpdatePixTransaction(ctx context.Context, tx pgx.Tx, pixTransaction model.PixTransaction) (int64, error){
	childLogger.Info().Str("func","UpdatePixTransaction").Interface("trace-resquest-id", ctx.Value("trace-request-id")).Send()

	// Trace
	span := tracerProvider.Span(ctx, "database.UpdatePixTransaction")
	defer span.End()

	// Query and execute
	query := `UPDATE pix_transaction
				SET status = $2,
					updated_at = $3
				WHERE id = $1`

	row, err := tx.Exec(ctx, query,	pixTransaction.ID,
									pixTransaction.Status,
									time.Now())
	if err != nil {
		return 0, errors.New(err.Error())
	}
	return row.RowsAffected(), nil
}