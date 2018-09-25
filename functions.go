package main

import (
	"fmt"
	"os"
	"strconv"
)

func getEnv(key, fallback string) string {
	returnVal := fallback
	if value, ok := os.LookupEnv(key); ok {
		returnVal = value
	}
	if returnVal == "" {
		panic(fmt.Sprintf("Unable to retrieve key: %s", key))
	}
	return returnVal
}

func getEnvNum(key string) int {
	if value, ok := os.LookupEnv(key); ok {
		returnVal, err := strconv.Atoi(value)
		if err != nil {
			panic(fmt.Sprintf("Unable to retrieve key: %s", key))
		}
		return returnVal
	}
	panic(fmt.Sprintf("Unable to retrieve key: %s", key))
}
