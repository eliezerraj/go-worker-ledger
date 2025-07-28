package configuration

import(
	"os"

	"github.com/joho/godotenv"
	"github.com/go-worker-ledger/internal/core/model"
)

// About get redis env var
func GetCacheEnv() model.CacheConfig {
	childLogger.Info().Str("func","GetCacheEnv").Send()

	err := godotenv.Load(".env")
	if err != nil {
		childLogger.Info().Err(err).Send()
	}
	
	var cacheConfig		model.CacheConfig

	if os.Getenv("CACHE_HOST") !=  "" {
		cacheConfig.Host = os.Getenv("CACHE_HOST")
	}
	
	if os.Getenv("CACHE_USERNAME") !=  "" {
		cacheConfig.Username = os.Getenv("CACHE_USERNAME")
	}
	
	if os.Getenv("CACHE_PASSWORD") !=  "" {
		cacheConfig.Password = os.Getenv("CACHE_PASSWORD")
	}

	return cacheConfig
}