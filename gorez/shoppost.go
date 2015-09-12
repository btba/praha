package main

import (
	"net/http"
)

type ShopPostVars struct {
	TourID int
}

func (s *Server) HandleShopPost(w http.ResponseWriter, r *http.Request) {
	// POST /reservations/shoppost TourID=562
	if r.Method != "POST" {
		http.Error(w, "Method must be POST", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var vars ShopPostVars
	if err := decoder.Decode(&vars, r.PostForm); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Read cartID from cookie.
	cartID, ok := readCartID(r)

	// If cookie, look up any items in this cart.
	var items []*CartItem
	if ok {
		var err error
		items, err = s.store.ListCartItems(cartID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// If no cookie or no items, create new cartID.
	if len(items) == 0 {
		cartID = newCartID()
		writeCartID(w, cartID)
	}

	// TODO: Check that tour has spots available.
	if item, ok := findTourInCart(items, vars.TourID); ok {
		if err := s.store.UpdateCartItem(cartID, item.ID, item.Quantity+1); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := s.store.AddCartItem(cartID, vars.TourID, 1); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/reservations/cart", http.StatusSeeOther)
}

func findTourInCart(items []*CartItem, tourID int) (*CartItem, bool) {
	for _, item := range items {
		if item.TourID == tourID {
			return item, true
		}
	}
	return nil, false
}
