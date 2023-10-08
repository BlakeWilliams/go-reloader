package main

import "net/http"

func main() {
	err := http.ListenAndServe(":3012", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello from test server"))
	}))

	if err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
