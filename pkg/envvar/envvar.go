package envvar

import (
	"fmt"
	"os"
)

// Get is just a wrapper for os.Getenv
func Get(key string) string {
	return os.Getenv(key)
}

// GetOrDefault returns the environment variable or the default if its not available
func GetOrDefault(key string, defaultValue string) string {
	return getOrDefault(key, func(key string) string {
		return defaultValue
	})
}

// GetOrPanic returns the environment variable or panics if its not available
func GetOrPanic(key string) string {
	return getOrDefault(key, func(key string) string {
		panic(fmt.Sprintf("environment variable '%s' couldn't be found", key))
	})
}

func getOrDefault(key string, defaultFn func(string) string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}

	return defaultFn(key)
}

// CheckEnvVarExists returns if a environment variable exists
func CheckEnvVarExists(existsKey string) bool {
	_, found := os.LookupEnv(existsKey)
	return found
}
