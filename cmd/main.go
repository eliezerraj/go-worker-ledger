package main

import(
	"time"
	"context"
	"sync"
	"crypto/tls"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/go-worker-ledger/internal/infra/configuration"
	"github.com/go-worker-ledger/internal/core/model"
	"github.com/go-worker-ledger/internal/core/service"
	"github.com/go-worker-ledger/internal/adapter/database"
	"github.com/go-worker-ledger/internal/adapter/event"
	"github.com/go-worker-ledger/internal/infra/server"

	go_core_api "github.com/eliezerraj/go-core/api"
	go_core_pg "github.com/eliezerraj/go-core/database/pg"
	go_core_cache "github.com/eliezerraj/go-core/cache/redis_cluster"

	redis "github.com/redis/go-redis/v9"
)

var(
	logLevel = 	zerolog.InfoLevel // zerolog.InfoLevel zerolog.DebugLevel
	childLogger = log.With().Str("component","go-worker-ledger").Str("package", "main").Logger()
	appServer			model.AppServer
	databaseConfig		go_core_pg.DatabaseConfig
	databasePGServer 	go_core_pg.DatabasePGServer
)

func init(){
	childLogger.Info().Str("func","init").Send()

	zerolog.SetGlobalLevel(logLevel)

	infoPod := configuration.GetInfoPod()
	configOTEL 		:= configuration.GetOtelEnv()
	databaseConfig 	:= configuration.GetDatabaseEnv() 
	apiService 		:= configuration.GetEndpointEnv()
	cacheConfig 	:= configuration.GetCacheEnv()
	kafkaConfigurations, topics := configuration.GetKafkaEnv() 

	appServer.InfoPod = &infoPod
	appServer.ConfigOTEL = &configOTEL
	appServer.DatabaseConfig = &databaseConfig
	appServer.ApiService = apiService
	appServer.CacheConfig = &cacheConfig
	appServer.KafkaConfigurations = &kafkaConfigurations
	appServer.Topics = topics
}

func main (){
	childLogger.Info().Str("func","main").Interface("appServer",appServer).Send()

	ctx := context.Background()

	// Open Database
	count := 1
	var err error
	for {
		databasePGServer, err = databasePGServer.NewDatabasePGServer(ctx, *appServer.DatabaseConfig)
		if err != nil {
			if count < 3 {
				childLogger.Error().Err(err).Msg("error open database... trying again !!")
			} else {
				childLogger.Error().Err(err).Msg("fatal error open Database aborting")
				panic(err)
			}
			time.Sleep(3 * time.Second) //backoff
			count = count + 1
			continue
		}
		break
	}

	// Open Valkey
	var redisClientCache 	go_core_cache.RedisClient
	var optRedisClient		redis.Options

	optRedisClient.Username = appServer.CacheConfig.Username
	optRedisClient.Password = appServer.CacheConfig.Password
	optRedisClient.Addr = appServer.CacheConfig.Host

	if true {
		optRedisClient.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	workerCache := redisClientCache.NewRedisClientCache(&optRedisClient)

	_, err = workerCache.Ping(context.Background())
	if err != nil {
		childLogger.Error().Err(err).Msg("Failed to ping redis")
	} else {
		childLogger.Info().Str("func","main").Msg("Valkey Ping Succesfull !!!")
	}

	// Database
	workerRepository := database.NewWorkerRepository(&databasePGServer)

	// Create a go-core api service for client http
	coreRestApiService := go_core_api.NewRestApiService()
	workerService := service.NewWorkerService(	*coreRestApiService, 
												workerRepository,
												workerCache,
												appServer.ApiService)
	
	// Kafka
	workerEvent, err := event.NewWorkerEvent(ctx, 
											appServer.Topics, 
											appServer.KafkaConfigurations)
	if err != nil {
		childLogger.Error().Err(err).Msg("error open kafka")
		panic(err)
	}

	serverWorker := server.NewServerWorker(workerService, workerEvent)

	var wg sync.WaitGroup
	wg.Add(1)
	go serverWorker.Consumer(ctx, &appServer, &wg)
	wg.Wait()
}