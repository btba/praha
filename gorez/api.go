package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (s *Server) HandleApiCartItems(w http.ResponseWriter, r *http.Request) {
	// PUT    /reservations/api/cartitems/<itemID> <quantity>
	// DELETE /reservations/api/cartitems/<itemID>
	itemID, err := strconv.Atoi(r.URL.Path[len("/reservations/api/cartitems/"):])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cartID, ok := readCartID(r)
	if !ok {
		http.Error(w, "No cart", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case "PUT":
		var quantity int32
		if err := json.NewDecoder(r.Body).Decode(&quantity); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.UpdateCartItem(cartID, int32(itemID), quantity); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "DELETE":
		if err := s.store.DeleteCartItem(cartID, int32(itemID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Method must be PUT or DELETE", http.StatusBadRequest)
		return
	}
}
