package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/miekg/dns"
	"golang.org/x/net/idna"

	"github.com/im-kulikov/resolvex/internal/domain"
)

type ResponseItem struct {
	Domain string    `json:"domain"`
	Record []string  `json:"record"`
	Expire time.Time `json:"expire"`
}

type ResponseList struct {
	List []ResponseItem `json:"list"`
}

type ErrorResponse struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
}

type Response struct {
	*ErrorResponse
	*ResponseItem
	*ResponseList
}

type ErrorHandler func(http.ResponseWriter, *http.Request) error

func validateDomain(domain string) error {
	if domain == "" {
		return errors.New("domain is required")
	}

	if _, err := idna.Lookup.ToASCII(domain); err != nil {
		return err
	}

	if _, err := net.LookupHost(domain); err != nil {
		return err
	}

	_, err := dns.Exchange(
		&dns.Msg{Question: []dns.Question{{Name: domain + ".", Qtype: dns.TypeA}}},
		"1.1.1.1:53",
	)

	return err
}

func (s *server) listCacheItems(w http.ResponseWriter, _ *http.Request) error {
	var result ResponseList
	for rec := range s.List() {
		result.List = append(result.List, ResponseItem{
			Domain: rec.Domain,
			Record: rec.Record,
			Expire: rec.Expire,
		})
	}

	return json.NewEncoder(w).Encode(Response{ResponseList: &result})
}

func (s *server) createCacheItem(w http.ResponseWriter, r *http.Request) error {
	var item ResponseItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		return err
	}

	if err := domain.Validate(item.Domain); err != nil {
		s.ErrorContext(r.Context(), "domain invalid",
			logger.String("domain", item.Domain),
			logger.Err(err))

		w.WriteHeader(http.StatusBadRequest)

		return json.NewEncoder(w).Encode(Response{
			ErrorResponse: &ErrorResponse{
				Code:        "400",
				Message:     "Invalid domain",
				Description: err.Error(),
			},
		})
	}

	if err := s.Create(item.Domain); err != nil {
		s.ErrorContext(r.Context(), "could not create domain",
			logger.String("domain", item.Domain),
			logger.Err(err))

		w.WriteHeader(http.StatusBadRequest)

		return json.NewEncoder(w).Encode(Response{
			ErrorResponse: &ErrorResponse{
				Code:    "400",
				Message: "domain exists",
			},
		})
	}

	w.WriteHeader(http.StatusCreated)

	return nil
}

func (s *server) updateCacheItem(w http.ResponseWriter, r *http.Request) error {
	oldDomain := r.PathValue("domain")

	var item ResponseItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		return err
	} else if err = validateDomain(item.Domain); err != nil {
		s.ErrorContext(r.Context(), "domain is wrong",
			logger.String("domain", item.Domain),
			logger.Err(err))

		w.WriteHeader(http.StatusBadRequest)

		return json.NewEncoder(w).Encode(Response{
			ErrorResponse: &ErrorResponse{
				Code:        "400",
				Message:     "Invalid domain",
				Description: err.Error(),
			},
		})
	}

	if err := s.Update(oldDomain, item.Domain); err != nil {
		s.ErrorContext(r.Context(), "could not update domain",
			logger.String("domain", item.Domain),
			logger.Err(err))

		w.WriteHeader(http.StatusBadRequest)

		return json.NewEncoder(w).Encode(Response{
			ErrorResponse: &ErrorResponse{
				Code:    "400",
				Message: "something went wrong",
			},
		})
	}

	w.WriteHeader(http.StatusAccepted)

	return nil
}

func (s *server) deleteCacheItem(w http.ResponseWriter, r *http.Request) error {
	value := r.PathValue("domain")

	if err := s.Delete(value); err != nil {
		s.ErrorContext(r.Context(), "could not update domain",
			logger.String("domain", value),
			logger.Err(err))

		w.WriteHeader(http.StatusNotFound)

		return json.NewEncoder(w).Encode(Response{
			ErrorResponse: &ErrorResponse{
				Code:    "404",
				Message: "Domain not found",
			},
		})
	}

	w.WriteHeader(http.StatusAccepted)

	return nil
}

// wrapErrorHandler оборачивает ErrorHandler, чтобы обрабатывать ошибки.
func wrapErrorHandler(handler ErrorHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		if err = handler(w, r); err == nil {
			return
		}

		// Преобразовать ошибку в ErrorResponse и отправить её клиенту
		w.WriteHeader(http.StatusInternalServerError)

		errorResponse := ErrorResponse{
			Code:    "500",
			Message: err.Error(),
		}

		if jsonErr := json.NewEncoder(w).Encode(errorResponse); jsonErr != nil {
			http.Error(w, jsonErr.Error(), http.StatusInternalServerError)
		}
	}
}

func (s *server) attach(srv *http.Server) {
	mux := http.NewServeMux()

	mux.Handle("GET /", http.FileServer(content))
	mux.HandleFunc("GET /api", wrapErrorHandler(s.listCacheItems))
	mux.HandleFunc("POST /api", wrapErrorHandler(s.createCacheItem))
	mux.HandleFunc("PUT /api/{domain}/", wrapErrorHandler(s.updateCacheItem))
	mux.HandleFunc("DELETE /api/{domain}/", wrapErrorHandler(s.deleteCacheItem))

	srv.Handler = mux
}
