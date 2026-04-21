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
	"github.com/nickplusdrums/chirpy/internal/auth"
)

type User struct {
	ID			uuid.UUID	`json:"id"`
	CreatedAt	time.Time	`json:"created_at"`
	UpdatedAt	time.Time	`json:"updated_at"`
	Email		string		`json:"email"`
	IsChirpyRed	bool		`json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type apiConfig struct {
	fileserverHits	atomic.Int32
	dbQuery			*database.Queries
	platform		string
	jwtSecret		string
	polkaAPI		string
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
		Password	string `json:"password"`
		Email		string `json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error in decoding: %v", err)
		w.WriteHeader(500)
		return
	}
	if params.Password == "" {
		respondWithError(w, 500, "Password field must not be blank.")
		return
	}

	hp, err := auth.HashPassword(params.Password)
	
	if err != nil {
		respondWithError(w, 500, "Failed to Call Hash Password")
		return
	}

	user, err := cfg.dbQuery.CreateUser(r.Context(), database.CreateUserParams{
		Email:			params.Email,
		HashedPassword: hp,
	})
	
	if err != nil {
		log.Printf("Error in running query: %v", err)
		w.WriteHeader(500)
		return
	}

	responseUser := User{
		ID:				user.ID,
		CreatedAt:		user.CreatedAt,
		UpdatedAt:		user.UpdatedAt,
		Email:			user.Email,
		IsChirpyRed:	user.IsChirpyRed,
	}
	respondWithJson(w, 201, responseUser)
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {

	cfg.fileserverHits.Store(0)
	if cfg.platform != "dev" {
		respondWithError(w, 403, "You are not an admin.")
		return
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
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Decode Error: %v", err)
		respondWithError(w, 500, "Failed to Decode")
		return
	}
	
	// lets validate the user first

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "Failed to call GetBearerToken")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Failed to call Validate JWT")
		return
	}
	
	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	} else {
		chirp, err := cfg.dbQuery.CreateChirp(r.Context(), database.CreateChirpParams{
			Body:   cleanString(params.Body),
			UserID: userID,
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
			UserID:    userID,
		}
		respondWithJson(w, 201, responseChirp)
	}
}

func (cfg *apiConfig) handlerGetChirps(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.dbQuery.GetChirps(r.Context())
	if err != nil {
		log.Printf("Error! %v", err)
		respondWithError(w, 500, "Failed to run Query")
		return
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

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	
	chirpIDstring := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDstring)
	if err != nil {
		log.Printf("Error: %v", err)
		respondWithError(w, 500, "Not a valid Chirp ID")
		return
	}

	chirp, err := cfg.dbQuery.GetChirp(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 404, "chirp not found.")
			return
		} else {
			log.Printf("Error! %v", err)
			respondWithError(w, 500, "Failed to run Query")
			return
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

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password	string	`json:"password"`
		Email		string	`json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 404, "User not found")
			return
		}
		respondWithError(w, 500, "Failed to decode JSON")
		return
	}

	user, err := cfg.dbQuery.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 404, "User not found")
			return
		}
		respondWithError(w, 500, "Failed to run Query")
		return
	}
	match, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		respondWithError(w, 500, "Failed to Run auth.CheckPasswordHash")
		return
	}

	if !match {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}
	
	accessExpDuration := time.Duration(60) * time.Minute

	accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, accessExpDuration)
	if err != nil {
		respondWithError(w, 500, "Error making Access JWT")
		return
	}
	refreshToken := auth.MakeRefreshToken()

	now := time.Now()
	
	err = cfg.dbQuery.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:		refreshToken,
		UserID:		user.ID,
		ExpiresAt:	now.AddDate(0, 0, 60),
	})
	if err != nil {
		respondWithError(w, 500, "Failed to create refresh token")
		log.Printf("CreateRefreshToken error: %v", err)
		return
	}

	type response struct {
		ID				uuid.UUID	`json:"id"`
		CreatedAt		time.Time	`json:"created_at"`
		UpdatedAt		time.Time	`json:"updated_at"`
		Email			string		`json:"email"`
		AccessToken		string		`json:"token"`
		RefreshToken	string		`json:"refresh_token"`
		IsChirpyRed		bool		`json:"is_chirpy_red"`
	}

	returnUser := response {
		ID:				user.ID,
		CreatedAt:		user.CreatedAt,
		UpdatedAt:		user.UpdatedAt,
		Email:			user.Email,
		AccessToken:	accessToken,
		RefreshToken:	refreshToken,
		IsChirpyRed:	user.IsChirpyRed,
	}
	respondWithJson(w, 200, returnUser)
}

func (cfg *apiConfig) handlerAPIRefresh(w http.ResponseWriter, r *http.Request) {

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "No Authorization Header")
		return
	}
	user, err := cfg.dbQuery.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 401, "No User")
			return
		}
		respondWithError(w, 500, "Failed to run query GetUserFromRefreshToken")
		return
	}
	accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		respondWithError(w, 500, "Error in MakeJWT")
		return
	}
	respondWithJson(w, 200, struct {
		Token string `json:"token"`
	}{
		Token: accessToken,
	})
}

func (cfg *apiConfig) handlerAPIRevoke(w http.ResponseWriter, r *http.Request) {

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "No Authorization Header")
		return
	}
	
	err = cfg.dbQuery.RevokeRefreshToken(r.Context(), refreshToken)

	if err != nil {
		respondWithError(w, 500, "Failed to RevokeRefreshToken")
		return
	}

	w.WriteHeader(204)
}

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "No Authorization Header")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Failed to call Validate JWT")
		return
	}

	type parameters struct {
		Email		string `json:"email"`
		Password	string `json:"password"`
	}

	params := parameters{}

	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 500, "Could not Decode")
		return
	}

	hp, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 500, "Could not Hash Password")
		return
	}

	user, err := cfg.dbQuery.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:				userID,
		Email:			params.Email,
		HashedPassword: hp,
	})
	if err != nil {
		respondWithError(w, 500, "Failed to call Update User")
		return
	}

	respondWithJson(w, 200, struct{
		ID			uuid.UUID	`json:"id"`
		CreatedAt	time.Time	`json:"created_at"`
		UpdatedAt	time.Time	`json:"updated_at"`
		Email		string		`json:"email"`
		IsChirpyRed	bool		`json:"is_chirpy_red"`
	}{
		ID: user.ID,
		CreatedAt:		user.CreatedAt,
		UpdatedAt:		user.UpdatedAt,
		Email:			user.Email,
		IsChirpyRed:	user.IsChirpyRed,
	})
}

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, 401, "No Authorization Header")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, 401, "Failed to call Validate JWT")
		return
	}
	
	chirpIDstring := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDstring)
	if err != nil {
		log.Printf("Error: %v", err)
		respondWithError(w, 500, "Not a valid Chirp ID")
		return
	}

	chirp, err := cfg.dbQuery.GetChirp(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, 404, "chirp not found.")
			return
		} else {
			log.Printf("Error! %v", err)
			respondWithError(w, 500, "Failed to run Query")
			return
		}
	}
	if chirp.UserID != userID {
		respondWithError(w, 403, "Not User of Chirp")
		return
	}
	err = cfg.dbQuery.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, 500, "Failed to run Query Delete Chirp")
		return
	}
	w.WriteHeader(204)
}

func (cfg *apiConfig) handlerPolkaWebhook(w http.ResponseWriter, r *http.Request) {

	token, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, 401, "Failed to Authorize")
		return
	}
	if cfg.polkaAPI != token {
		respondWithError(w, 401, "Not Authorized")
		return
	}

	type parameters struct {
		Event	string			`json:"event"`
		Data	struct {
			UserID uuid.UUID	`json:"user_id"`
		}						`json:"data"`
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "Failed to Decode Body")
		return
	}
	if params.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}
	_, err = cfg.dbQuery.UpgradeToRed(r.Context(), params.Data.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(204)
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
	cfg.jwtSecret = os.Getenv("JWT")
	cfg.polkaAPI = os.Getenv("POLKA_KEY")

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
	mux.HandleFunc("POST /api/login", cfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", cfg.handlerAPIRefresh)
	mux.HandleFunc("POST /api/revoke", cfg.handlerAPIRevoke)
	mux.HandleFunc("PUT /api/users", cfg.handlerUpdateUser)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.handlerDeleteChirp)
	mux.HandleFunc("POST /api/polka/webhooks", cfg.handlerPolkaWebhook)
	err = server.ListenAndServe()
	if err != nil {
		return
	}
}
