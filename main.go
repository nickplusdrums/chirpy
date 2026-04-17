package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"errors"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/nickplusdrums/chirpy/internal/database"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQuery        *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func handlerReadiness(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	responseWriter.WriteHeader(http.StatusOK)
	responseWriter.Write([]byte("OK"))
}

func (cfg *apiConfig) handlerHits(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	responseWriter.WriteHeader(http.StatusOK)
	responseWriter.Write([]byte(fmt.Sprintf(`
						<html>
							<body>
								<h1>Welcome, Chirpy Admin</h1>
								<p>Chirpy has been visited %d times!</p>
							</body>
						</html>
						`, cfg.fileserverHits.Load())))
}

func cleanString(s string) string {
	words := strings.Split(s, " ")
	for i := 0; i < len(words); i++ {
		w := strings.ToLower(words[i])
		if w == "kerfuffle" || w == "sharbert" || w == "fornax" {
			words[i] = "****"
		}
	}
	return strings.Join(words, " ")
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errBody struct {
		Err string `json:"error"`
	}
	respBody := errBody{
		Err: msg,
	}
	respondWithJson(w, code, respBody)
}

func respondWithJson(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error in decoding: %v", err)
		w.WriteHeader(500)
		return
	}
	user, err := cfg.dbQuery.CreateUser(r.Context(), params.Email)
	if err != nil {
		log.Printf("Error in running query: %v", err)
		w.WriteHeader(500)
		return
	}
	responseUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}
	respondWithJson(w, 201, responseUser)
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {

	cfg.fileserverHits.Store(0)
	if cfg.platform != "dev" {
		respondWithError(w, 403, "You are not an admin.")
	} else {
		err := cfg.dbQuery.ResetUsers(r.Context())
		if err != nil {
			w.WriteHeader(500)
			return
		}
	}
	w.WriteHeader(200)
}

func (cfg *apiConfig) handlerChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Decode Error: %v", err)
		respondWithError(w, 500, "Failed to Decode")
		return
	}

	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	} else {
		chirp, err := cfg.dbQuery.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   cleanString(params.Body),
			UserID: params.UserID,
		})
		if err != nil {
			log.Printf("Error! %v", err)
			respondWithError(w, 500, "Failed to run Query")
			return
		}
		responseChirp := Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}
		respondWithJson(w, 201, responseChirp)
	}
}

func (cfg apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.dbQuery.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error! %v", err)
		respondWithError(w, 500, "Failed to run Query")
	}
	responseChirps := []Chirp{}
	for _, chirp := range chirps {
		responseChirps = append(responseChirps, Chirp{
			ID:			chirp.ID,
			CreatedAt:	chirp.CreatedAt,
			UpdatedAt:	chirp.UpdatedAt,
			Body:		chirp.Body,
			UserID:		chirp.UserID,
		})
	}
	respondWithJson(w, 200, responseChirps)
}

func (cfg apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	
	chirpIDstring := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDstring)
	if err != nil {
		log.Printf("Error: %v", err)
		respondWithError(w, 500, "Not a valid Chirp ID")
	}

	chirp, err := cfg.dbQuery.GetChirp(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 404, "Chirp not found.")
		} else {
			log.Printf("Error! %v", err)
			respondWithError(w, 500, "Failed to run Query")
		}
	}
	responseChirp := Chirp{
		ID:			chirp.ID,
		CreatedAt:	chirp.CreatedAt,
		UpdatedAt:	chirp.UpdatedAt,
		Body:		chirp.Body,
		UserID:		chirp.UserID,
	}
	respondWithJson(w, 200, responseChirp)
}

func main() {

	godotenv.Load()
	dbURL := os.Getenv("DB_URL")

	cfg := apiConfig{}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println("ERROR OPENING DATABASE")
	}
	cfg.dbQuery = database.New(db)
	cfg.platform = os.Getenv("PLATFORM")

	mux := http.NewServeMux()
	var server http.Server
	server.Addr = ":8080"
	server.Handler = mux
	mux.Handle("/app/", http.StripPrefix("/app", cfg.middlewareMetricsInc(http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /api/healthz", handlerReadiness)
	mux.HandleFunc("GET /admin/metrics", cfg.handlerHits)
	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)
	mux.HandleFunc("POST /api/users", cfg.handlerCreateUser)
	mux.HandleFunc("POST /api/chirps", cfg.handlerChirp)
	mux.HandleFunc("GET /api/chirps", cfg.handlerGetChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.handlerGetChirp)
	err = server.ListenAndServe()
	if err != nil {
		return
	}
}
