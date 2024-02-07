package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"ewallet/internal/app/store"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"net/http"
)

// Server struct holds the state of the server
type Server struct {
	config *Config
	router *mux.Router
	store  *store.Store
}

func New(config *Config) *Server {
	return &Server{
		config: config,
	}
}

func (s *Server) Start() error {
	s.configureRouter()
	if err := s.configureStore(); err != nil {
		return err
	}

	return http.ListenAndServe(s.config.BindAddress, s.router)
}

// configureRouter returns the HTTP handler for the server
func (s *Server) configureRouter() {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/wallet", s.createWalletHandler).Methods("POST")
	r.HandleFunc("/api/v1/wallet/{walletId}/send", s.sendMoneyHandler).Methods("POST")
	r.HandleFunc("/api/v1/wallet/{walletId}/history", s.getTransactionHistoryHandler).Methods("GET")
	r.HandleFunc("/api/v1/wallet/{walletId}", s.getWalletStatusHandler).Methods("GET")
	s.router = r
}

func (s *Server) configureStore() error {
	st := store.New(s.config.StoreCfg)
	if err := st.Open(); err != nil {
		return err
	}

	s.store = st
	return nil
}

// createWalletHandler handles the creation of a new wallet
func (s *Server) createWalletHandler(w http.ResponseWriter, r *http.Request) {
	newUuid := uuid.New().String()
	fmt.Println(newUuid)
	var db = s.store.GetWalletDB()
	wallet, err := db.Create(newUuid, 100)
	if err != nil {
		http.Error(w, "Failed to create wallet", http.StatusInternalServerError)
		return
	}

	response := struct {
		ID      string  `json:"id"`
		Balance float64 `json:"balance"`
	}{
		ID:      wallet.ID,
		Balance: wallet.Balance,
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// getWalletStatusHandler handles retrieving the current status of a wallet
func (s *Server) getWalletStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	walletID := vars["walletId"]

	// Получение состояния кошелька
	wallet, err := s.store.WalletDB.CheckStatus(walletID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "wallet not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal Server Error", http.StatusInternalServerError)
		return
	}

	// Формирование ответа в формате JSON с ID и балансом созданного кошелька
	response := struct {
		ID      string  `json:"id"`
		Balance float64 `json:"balance"`
	}{
		ID:      wallet.ID,
		Balance: wallet.Balance,
	}

	// Установка заголовка Content-Type для указания формата ответа
	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// sendMoneyHandler handles money transfer between wallets
func (s *Server) sendMoneyHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	from := vars["walletId"]

	var request struct {
		To     string  `json:"to"`
		Amount float64 `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var db = s.store.GetTransactionDB()
	err := db.TransferMoney(from, request.To, request.Amount)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// Если кошелек не найден, возвращаем статус ответа 404
			http.Error(w, "sender wallet not found", http.StatusNotFound)
		case errors.Is(err, errors.New("there are not enough funds")):
			// Если недостаточно средств на кошельке, возвращаем статус ответа 400
			http.Error(w, "not enough funds", http.StatusBadRequest)
		case errors.Is(err, errors.New("target wallet not found")):
			// Если целевой кошелек не найден, возвращаем статус ответа 404
			http.Error(w, "target wallet not found", http.StatusNotFound)
		default:
			// В случае других ошибок, возвращаем статус ответа 500
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Если успешно, возвращаем статус ответа 200
	w.WriteHeader(http.StatusOK)

}

// getTransactionHistoryHandler handles retrieving transaction history for a wallet
func (s *Server) getTransactionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	from := vars["walletId"]
	var db = s.store.GetTransactionDB()

	transactions, err := db.GetWalletTransactions(from)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "sender wallet not found", http.StatusNotFound)
			return
		}
		return
	}

	// Return the transaction history as JSON
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(transactions)
	if err != nil {
		return
	}

}
