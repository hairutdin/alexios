package main

import (
	"fmt"
	"net/http"
)

func main() {
	parseFlags()

	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	fmt.Println("Running server on", flagRunAddr)
	return http.ListenAndServe(":8080", http.HandlerFunc(webhook))
}

func webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`
	{
		"response": {
			"text": "Sorry, I can't do anything at the moment"
		},
		"version": "1.0"
	}
	`))
}
