package server

import (
	"context"
	"sync"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/go-worker-ledger/internal/core/service"
	"github.com/go-worker-ledger/internal/adapter/event"
	"github.com/go-worker-ledger/internal/core/model"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/contrib/propagators/aws/xray"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	go_core_observ "github.com/eliezerraj/go-core/observability"
	go_core_event "github.com/eliezerraj/go-core/event/kafka" 
)

var ( 
	childLogger = log.With().Str("component","go-worker-ledger").Str("package","internal.infra.server").Logger()
	tracerProvider go_core_observ.TracerProvider
	consumerWorker go_core_event.ConsumerWorker
	infoTrace go_core_observ.InfoTrace
	tracer 			trace.Tracer
)

type ServerWorker struct {
	workerService 	*service.WorkerService
	workerEvent 	*event.WorkerEvent
}

// Set a trace-i inside the context
func setContextTraceId(ctx context.Context, trace_id string) context.Context {
	childLogger.Info().Str("func","setContextTraceId").Interface("trace_id", trace_id).Send()

	var traceUUID string

	if trace_id == "" {
		traceUUID = uuid.New().String()
		trace_id = traceUUID
	}

	ctx = context.WithValue(ctx, "trace-request-id",  trace_id  )
	return ctx
}

// About create a worrker event
func NewServerWorker(workerService *service.WorkerService, workerEvent *event.WorkerEvent ) *ServerWorker {
	childLogger.Info().Str("func","NewServerWorker").Send()

	return &ServerWorker{
		workerService: workerService,
		workerEvent: workerEvent,
	}
}

type kafkaHeaderCarrier []kafka.Header

// About consume event kafka
func (s *ServerWorker) Consumer(ctx context.Context, appServer *model.AppServer ,wg *sync.WaitGroup ) {
	childLogger.Info().Str("func","Consumer").Send()

	// otel
	infoTrace.PodName = appServer.InfoPod.PodName
	infoTrace.PodVersion = appServer.InfoPod.ApiVersion
	infoTrace.ServiceType = "k8-workload"
	infoTrace.Env = appServer.InfoPod.Env
	infoTrace.AccountID = appServer.InfoPod.AccountID

	tp := tracerProvider.NewTracerProvider(	ctx, 
											appServer.ConfigOTEL, 
											&infoTrace)
	
	if tp != nil {
		otel.SetTextMapPropagator(xray.Propagator{})
		otel.SetTracerProvider(tp)
		tracer = tp.Tracer(appServer.InfoPod.PodName)
	}

	// handle defer
	defer func() {
		// check otel is on
		if tp != nil {
			err := tp.Shutdown(ctx)
			if err != nil{
				childLogger.Info().Err(err).Send()
			}
		}
		childLogger.Info().Msg("closing consumer waiting please !!!")
		defer wg.Done()
	}()

	messages := make(chan go_core_event.Message)

	go s.workerEvent.WorkerKafka.Consumer(s.workerEvent.Topics, messages)

	for msg := range messages {
		childLogger.Info().Msg("=============== MSG FROM KAFKA ==================")
		childLogger.Info().Interface("msg",msg).Send()
		childLogger.Info().Msg("=============== MSG FROM KAFKA ==================")
		
		// Marshall payload	
		var pixTransaction model.PixTransaction
		json.Unmarshal([]byte(msg.Payload), &pixTransaction)

		// valid the headers, if there isnt a traceid it will be created
		var header string
		if (*msg.Header)["trace-request-id"] != "" {
			header = (*msg.Header)["trace-request-id"]
		}

		ctx = setContextTraceId(ctx, header)

		//Trace
		// Convert headers to carrier
		carrier := propagation.MapCarrier{}

		carrier["X-Amzn-Trace-Id"] = (*msg.Header)["X-Amzn-Trace-Id"]
		carrier["TraceID"] = (*msg.Header)["TraceID"]
		carrier["SpanID"] = (*msg.Header)["SpanID"]

		parentCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
		ctx, span := tracer.Start(parentCtx, appServer.InfoPod.PodName)

		// call service
		_, err := s.workerService.PixTransactionAsync(ctx, &pixTransaction)
		if err != nil {
			childLogger.Error().Err(err).Interface("trace-resquest-id", ctx.Value("trace-request-id")).Send()
			childLogger.Info().Interface("trace-resquest-id", ctx.Value("trace-request-id")).Msg("ROLLBACK!!!!")
		} else {
			s.workerEvent.WorkerKafka.Commit()
			childLogger.Info().Interface("trace-resquest-id", ctx.Value("trace-request-id")).Msg("COMMIT!!!!")
		}

		span.End()
	}
}