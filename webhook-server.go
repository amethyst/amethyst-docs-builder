package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi"
)

var secret string
var scriptPath string

func main() {
	if _, ok := os.LookupEnv("SECRET"); !ok {
		log.Fatal("secret env variable is not set!")
	}
	secret = os.Getenv("SECRET")

	port := "3000"
	if val, ok := os.LookupEnv("PORT"); ok {
		port = val
	}

	scriptPath = "./run.sh"
	if val, ok := os.LookupEnv("SCRIPT"); ok {
		scriptPath = val
	}

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("up"))
	})

	r.Post("/trigger", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]interface{}
		if r.Body == nil {
			http.Error(w, "empty body", 400)
			return
		}

		err := json.NewDecoder(r.Body).Decode(&b)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

		fmt.Printf("received: %v\n", b)
	})

	http.ListenAndServe(fmt.Sprintf(":%s", port), r)
}

func validateSecret(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("validating secret: %v\n", secret)
		next.ServeHTTP(w, r)
	})
}
