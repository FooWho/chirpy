package main

import (
	"net/http"
	"strconv"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (apc *apiConfig) GetHits() int {
	return int(apc.fileserverHits.Load())
}

func (apc *apiConfig) Reset() {
	apc.fileserverHits = atomic.Int32{}
}

func (apc *apiConfig) IncrementHits() {
	apc.fileserverHits.Add(int32(1))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.IncrementHits()
		next.ServeHTTP(w, r)
	})
}

func main() {
	myCfg := &apiConfig{fileserverHits: atomic.Int32{}}

	mux := http.NewServeMux()
	mux.Handle("/app/",
		myCfg.middlewareMetricsInc(
			http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		hits := strconv.Itoa(myCfg.GetHits())
		w.Write([]byte("Hits: " + hits))
	})

	mux.HandleFunc("POST /api/reset", func(w http.ResponseWriter, r *http.Request) {
		myCfg.Reset()
	})

	s := &http.Server{Addr: ":8080", Handler: mux}
	s.ListenAndServe()
}
