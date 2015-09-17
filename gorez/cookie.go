package main

import (
	"math/rand"
	"net/http"
	"strconv"
)

const (
	cartIDCookieName = "BTBACartID"
)

func newCartID() int32 {
	// NB: Very small possibility of collision, so you would get an
	// existing cart.  It's ok.
	// TODO: Use an int64.
	return rand.Int31() & 0x7FFFFFFF
}

func readCartID(r *http.Request) (int32, bool) {
	cookie, err := r.Cookie(cartIDCookieName)
	if err != nil {
		return 0, false
	}
	cartID, err := strconv.ParseInt(cookie.Value, 16, 32)
	if err != nil {
		return 0, false
	}
	return int32(cartID), true
}

func writeCartID(w http.ResponseWriter, cartID int32) {
	http.SetCookie(w, &http.Cookie{
		Name:  cartIDCookieName,
		Value: strconv.FormatInt(int64(cartID), 16),
		Path:  "/",
	})
}
