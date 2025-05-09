package service

import(
	"fmt"
	"time"
	"context"
	"net/http"
	"encoding/json"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/go-worker-ledger/internal/adapter/database"
	"github.com/go-worker-ledger/internal/core/model"
	"github.com/go-worker-ledger/internal/core/erro"
	go_core_observ "github.com/eliezerraj/go-core/observability"
	go_core_api "github.com/eliezerraj/go-core/api"
)

var childLogger = log.With().Str("component","go-worker-ledger").Str("package","internal.core.service").Logger()
var tracerProvider go_core_observ.TracerProvider
var apiService go_core_api.ApiService

type WorkerService struct {
	goCoreRestApiService	go_core_api.ApiService
	workerRepository *database.WorkerRepository
	apiService		[]model.ApiService
}

// About create a new worker service
func NewWorkerService(	goCoreRestApiService	go_core_api.ApiService,	
						workerRepository *database.WorkerRepository, 
						apiService	[]model.ApiService) *WorkerService{
	childLogger.Debug().Str("func","NewWorkerService").Send()

	return &WorkerService{
		goCoreRestApiService: goCoreRestApiService,
		workerRepository: workerRepository,
		apiService: apiService,
	}
}

// About handle/convert http status code
func errorStatusCode(statusCode int, serviceName string) error{
	childLogger.Info().Str("func","errorStatusCode").Interface("serviceName", serviceName).Interface("statusCode", statusCode).Send()
	var err error
	switch statusCode {
		case http.StatusUnauthorized:
			err = erro.ErrUnauthorized
		case http.StatusForbidden:
			err = erro.ErrHTTPForbiden
		case http.StatusNotFound:
			err = erro.ErrNotFound
		default:
			err = errors.New(fmt.Sprintf("service %s in outage", serviceName))
		}
	return err
}

func (s WorkerService) PixTransactionAsync(ctx context.Context, pixTransaction *model.PixTransaction) (*model.PixTransaction, error){
	childLogger.Info().Str("func","PixTransactionAsync").Interface("trace-resquest-id", ctx.Value("trace-request-id")).Interface("pixTransaction", pixTransaction).Send()

	//Trace
	span := tracerProvider.Span(ctx, "service.PixTransactionAsync")
	trace_id := fmt.Sprintf("%v", ctx.Value("trace-request-id") )

	// Get the database connection
	tx, conn, err := s.workerRepository.DatabasePGServer.StartTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.workerRepository.DatabasePGServer.ReleaseTx(conn)

	// Handle the transaction
	defer func() {
		if err != nil {
			childLogger.Info().Interface("trace-request-id", trace_id ).Msg("ROLLBACK TX !!!")
			tx.Rollback(ctx)
		} else {
			childLogger.Info().Interface("trace-request-id", trace_id ).Msg("COMMIT TX !!!")
			tx.Commit(ctx)
		}	
		span.End()
	}()

	// ------------------------  STEP-1 ----------------------------------//
	childLogger.Info().Str("func","PixTransactionAsync").Msg("===> STEP - 01 (ACCOUNT FROM) <===")

	// prepare headers
	headers := map[string]string{
		"Content-Type":  	"application/json;charset=UTF-8",
		"X-Request-Id": 	trace_id,
		"x-apigw-api-id": 	s.apiService[0].XApigwApiId,
		"Host": 			s.apiService[0].HostName,
	}
	httpClient := go_core_api.HttpClient {
		Url: 	s.apiService[0].Url + "/get/" + pixTransaction.AccountFrom.AccountID,
		Method: s.apiService[0].Method,
		Timeout: 15,
		Headers: &headers,
	}

	res_payload, statusCode, err := apiService.CallRestApiV1(ctx,
															s.goCoreRestApiService.Client,
															httpClient, 
															nil)
	if err != nil {
		return nil, errorStatusCode(statusCode, s.apiService[0].Name)
	}

	jsonString, err  := json.Marshal(res_payload)
	if err != nil {
		return nil, errors.New(err.Error())
    }
	var account_from_parsed model.Account
	json.Unmarshal(jsonString, &account_from_parsed)

	// ------------------------  STEP-2 ----------------------------------//
	childLogger.Info().Str("func","PixTransactionAsync").Msg("===> STEP - 02 (ACCOUNT TO) <===")

	httpClient = go_core_api.HttpClient {
		Url: 	s.apiService[0].Url + "/get/" + pixTransaction.AccountTo.AccountID,
		Method: s.apiService[0].Method,
		Timeout: 15,
		Headers: &headers,
	}

	res_payload, statusCode, err = apiService.CallRestApiV1(ctx,
															s.goCoreRestApiService.Client,
															httpClient, 
															nil)
	if err != nil {
		return nil, errorStatusCode(statusCode, s.apiService[0].Name)
	}

	jsonString, err  = json.Marshal(res_payload)
	if err != nil {
		return nil, errors.New(err.Error())
    }
	var account_to_parsed model.Account
	json.Unmarshal(jsonString, &account_to_parsed)
	// ------------------------  STEP-3 ----------------------------------//
	childLogger.Info().Str("func","PixTransactionAsync").Msg("===> STEP - 03 (LEDGER) <===")
	
	// prepare headers
	headers = map[string]string{
		"Content-Type":  	"application/json;charset=UTF-8",
		"X-Request-Id": 	trace_id,
		"x-apigw-api-id": 	s.apiService[1].XApigwApiId,
		"Host": 			s.apiService[1].HostName,
	}
	httpClient = go_core_api.HttpClient {
		Url: 	s.apiService[1].Url + "/movimentTransaction",
		Method: s.apiService[1].Method,
		Timeout: 15,
		Headers: &headers,
	}

	moviment := model.Moviment{	AccountFrom:	pixTransaction.AccountFrom,
								AccountTo:	&pixTransaction.AccountTo,
								Type:	"WIRE_TRANSFER",
								Currency:	pixTransaction.Currency,
								Amount:	pixTransaction.Amount,
	}

	_, statusCode, err = apiService.CallRestApiV1(ctx,
												s.goCoreRestApiService.Client,
												httpClient, 
												moviment)
	if err != nil {
		return nil, errorStatusCode(statusCode, s.apiService[1].Name)
	}	

	// ------------------------  STEP-4 ----------------------------------//
	childLogger.Info().Str("func","PixTransactionAsync").Msg("===> STEP - 04 (UPDATE) <===")

	// setting status
	pixTransaction.AccountFrom = account_from_parsed
	pixTransaction.AccountTo = account_to_parsed
	pixTransaction.Status = "IN-QUEUE:CONSUMED"
	update := time.Now()
	pixTransaction.UpdatedAt = &update

	// update status payment
	res_update, err := s.workerRepository.UpdatePixTransaction(ctx, tx, *pixTransaction)
	if err != nil {
		return nil, err
	}
	if res_update == 0 {
		err = erro.ErrUpdate
		return nil, err
	}

	return pixTransaction, nil
}