package main

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Tour struct {
	ID   int64
	Code string
	Time time.Time
}

type CartItem struct {
	ID       int
	TourID   int
	Quantity int
}

type Store interface {
	ListOpenToursByCode() (map[string][]*Tour, error)
	ListCartItems(cartID int) ([]*CartItem, error)
	AddCartItem(cartID, tourID, quantity int) error
	UpdateCartItem(cartID, itemID, quantity int) error
}

type RemoteStore struct {
	db *sql.DB
}

func (s *RemoteStore) ListOpenToursByCode() (map[string][]*Tour, error) {
	rows, err := s.db.Query("SELECT TourID, TourCode, TourDateTime "+
		"FROM Master "+
		"WHERE TourDateTime >= ? AND Public <> 0 AND Cancelled = 0 AND Deleted = 0 "+
		"ORDER BY TourDateTime ASC",
		time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}
	toursByCode := make(map[string][]*Tour)
	for rows.Next() {
		var id int64
		var code sql.NullString
		var time time.Time
		if err := rows.Scan(&id, &code, &time); err != nil {
			return nil, err
		}
		toursByCode[code.String] = append(toursByCode[code.String], &Tour{
			ID:   id,
			Code: code.String,
			Time: time,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return toursByCode, nil
}

func (s *RemoteStore) ListCartItems(cartID int) ([]*CartItem, error) {
	rows, err := s.db.Query("SELECT ItemPos, TourID, RiderCount "+
		"FROM CartItems "+
		"WHERE CartItems.CartID = ?",
		cartID)
	if err != nil {
		return nil, err
	}
	var items []*CartItem
	for rows.Next() {
		var itemPos, tourID, riderCount int
		if err := rows.Scan(&itemPos, &tourID, &riderCount); err != nil {
			return nil, err
		}
		items = append(items, &CartItem{
			ID:       itemPos,
			TourID:   tourID,
			Quantity: riderCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *RemoteStore) AddCartItem(cartID, tourID, quantity int) error {
	_, err := s.db.Exec(
		"INSERT INTO CartItems (CartID, TourID, RiderCount) VALUES (?, ?, ?)",
		cartID, tourID, quantity)
	return err
}

func (s *RemoteStore) UpdateCartItem(cartID, itemID, quantity int) error {
	_, err := s.db.Exec(
		"UPDATE CartItems SET RiderCount = ? WHERE CartID = ? AND ItemPos = ?",
		quantity, cartID, itemID)
	return err
}
