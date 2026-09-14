package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

func (cfg *apiConfig) getMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
<html>
<body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
</body>
</html>
    `, cfg.getHits())
}

func (cfg *apiConfig) doReset(w http.ResponseWriter, r *http.Request) {

	if cfg.platform == "dev" {
		cfg.fileserverHits = atomic.Int32{}
		err := cfg.dbQueries.ResetUsers(r.Context())
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error resetting users table")
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	} else {
		respondWithError(w, http.StatusForbidden, "Forbidden")
	}
}

func (apc *apiConfig) getHits() int {
	return int(apc.fileserverHits.Load())
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
