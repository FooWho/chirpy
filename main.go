package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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

	mux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Content-Type", "text/html")
		w.WriteHeader(200)
		hits := myCfg.GetHits()
		hitString := fmt.Sprintf("<html>\n  <body>\n    <h1>Welcome, Chirpy Admin!</h1>\n    <p>Chirpy has been visited %d times!</p>\n  </body>\n</html>\n", hits)
		w.Write([]byte(hitString))
	})

	mux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		myCfg.Reset()
	})

	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Body string `json:"body"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			log.Printf("Error decoding parameters: %s", err)
			respondWithError(w, 500, "Error decoding parameters")
			return
		}
		if len(params.Body) <= 140 {
			params.Body = replaceBadWords(params.Body)
			respondWithJSON(w, 200, params.Body)
		} else {
			respondWithError(w, 400, "Chirp too long")
		}
	})

	s := &http.Server{Addr: ":8080", Handler: mux}
	s.ListenAndServe()
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type parameters struct {
		Error string `json:"error"`
	}

	params := parameters{}
	params.Error = msg

	dat, err := json.Marshal(params)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		respondWithError(w, 500, "Error marshalling json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	type returnVals struct {
		CleanedBody string `json:"cleaned_body"`
	}
	respBody := returnVals{}
	cleanedBody, ok := payload.(string)
	if !ok {
		fmt.Printf("blah")
	} else {
		respBody = returnVals{
			CleanedBody: cleanedBody,
		}
	}

	dat, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		respondWithError(w, 500, "Error marshalling json")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func replaceBadWords(s string) string {
	words := strings.Split(s, " ")
	for i, word := range words {
		if strings.ToLower(word) == "kerfuffle" ||
			strings.ToLower(word) == "sharbert" ||
			strings.ToLower(word) == "fornax" {
			words[i] = "****"
		}
	}
	cleanS := strings.Join(words, " ")
	return cleanS
}
