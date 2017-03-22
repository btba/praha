package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Tour struct {
	ID            int32
	Code          string
	Time          time.Time
	HeightsNeeded bool
}

type TourDetail struct {
	Tour
	LongName          string
	Price             float64
	NumSpotsRemaining int
}

type Store interface {
	GetTourDetailByID(tourID int32, maxRiders int) (*TourDetail, bool, error)
	CreateOrder(tourID int32, heights []int, total uint64, name, email, mobile, hotel, misc string) (int32, error)
	UpdateOrderPaymentRecorded(orderID int32) error
	UpdateOrderConfirmationSent(orderID int32) error
}

type RemoteStore struct {
	db *sql.DB
}

func (s *RemoteStore) GetTourDetailByID(tourID int32, maxRiders int) (*TourDetail, bool, error) {
	// Master.RiderLimit - SUM(OrderItems.Riders) will be NULL when Master.RiderLimit is NULL.
	rows, err := s.db.Query(
		"SELECT Master.TourID, "+
			"     Master.TourCode, "+
			"     Master.TourDateTime, "+
			"     Master.HeightsNeeded IS NOT NULL, "+
			"     MasterTourInfo.LongName, "+
			"     MasterTourInfo.Price, "+
			"     Master.RiderLimit - SUM(OrderItems.Riders) "+
			"FROM Master, MasterTourInfo, OrderItems "+
			"WHERE Master.TourID = ? AND Master.TourCode = MasterTourInfo.ShortCode AND OrderItems.TourID = ?",
		tourID, tourID)
	if err != nil {
		return nil, false, err
	}
	var tourDetail *TourDetail
	for rows.Next() {
		var (
			id                int32
			code              string
			time              mysql.NullTime
			heightsNeeded     bool
			longName          sql.NullString
			price             sql.NullFloat64
			numSpotsRemaining sql.NullInt64
		)
		if err := rows.Scan(&id, &code, &time, &heightsNeeded, &longName, &price, &numSpotsRemaining); err != nil {
			return nil, false, err
		}
		tourDetail = &TourDetail{
			Tour: Tour{
				ID:            id,
				Code:          code,
				Time:          time.Time,
				HeightsNeeded: heightsNeeded,
			},
			LongName: longName.String,
			Price:    price.Float64,
		}
		if numSpotsRemaining.Valid {
			tourDetail.NumSpotsRemaining = int(numSpotsRemaining.Int64)
		} else {
			tourDetail.NumSpotsRemaining = maxRiders
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return tourDetail, tourDetail != nil, nil
}

func priceString(total uint64) string {
	return fmt.Sprintf("%d", total/100)
}

func heightsString(heights []int) string {
	var s []string
	for _, h := range heights {
		s = append(s, fmt.Sprintf("%d", h))
	}
	return strings.Join(s, ", ")
}

func (s *RemoteStore) prepareCreateOrder(tx *sql.Tx, tourID int32, heights []int, total uint64, name, email, mobile, hotel, misc string) (int32, error) {
	result, err := tx.Exec(
		"INSERT INTO OrderMain (CustName, CustEmail, Hotel, Mobile, DatePlaced, Message, Heights) VALUES (?, ?, ?, ?, ?, ?, ?)",
		name, email, hotel, mobile, time.Now(), misc, heightsString(heights))
	if err != nil {
		return 0, err
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(
		"INSERT INTO OrderItems (OrderNum, TourID, Riders, Price, Method) VALUES (?, ?, ?, ?, ?)",
		orderID, tourID, len(heights), priceString(total), "STw")
	if err != nil {
		return 0, err
	}
	return int32(orderID), nil
}

func (s *RemoteStore) CreateOrder(tourID int32, heights []int, total uint64, name, email, mobile, hotel, misc string) (int32, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	orderID, err := s.prepareCreateOrder(tx, tourID, heights, total, name, email, mobile, hotel, misc)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return orderID, nil
}

func (s *RemoteStore) UpdateOrderPaymentRecorded(orderID int32) error {
	_, err := s.db.Exec(
		"UPDATE OrderMain SET Completed = true WHERE OrderNum = ?", orderID)
	return err
}

func (s *RemoteStore) UpdateOrderConfirmationSent(orderID int32) error {
	_, err := s.db.Exec(
		"UPDATE OrderItems SET ConfirmationSent = 1 WHERE OrderNum = ?", orderID)
	return err
}
