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
	ConfCode      string
	AutoConfirm   bool
	Full          bool
	Cancelled     bool
	HeightsNeeded bool
	Deleted       bool
}

type TourDetail struct {
	Tour
	LongName          string
	Price             float64
	NumSpotsRemaining int
}

type Rider struct {
	Gender string
	Height int
}

type Store interface {
	GetTourDetailByID(tourID int32, maxRiders int) (*TourDetail, bool, error)
	CreateOrder(tourID int32, numRiders int, riders []Rider, total uint64, name, email, mobile, hotel, misc string) (int32, error)
	UpdateOrderPaymentRecorded(orderID int32) error
	UpdateOrderConfirmationSent(orderID int32) error
}

type RemoteStore struct {
	db *sql.DB
}

func (s *RemoteStore) GetTourDetailByID(tourID int32, maxRiders int) (*TourDetail, bool, error) {
	var (
		id            int32
		code          sql.NullString
		time          mysql.NullTime
		confCode      sql.NullString
		autoConfirm   sql.NullBool
		full          sql.NullBool
		cancelled     sql.NullBool
		riderLimit    sql.NullInt64
		heightsNeeded sql.NullBool
		deleted       sql.NullBool
		longName      sql.NullString
		price         sql.NullFloat64
		numRiders     sql.NullInt64 // SUM() can return NULL
	)
	row := s.db.QueryRow(""+
		"SELECT Master.TourID, "+
		"    Master.TourCode, "+
		"    Master.TourDateTime, "+
		"    Master.ConfCode, "+
		"    Master.AutoConfirm <> 0, "+
		"    Master.TourFull, "+
		"    Master.Cancelled, "+
		"    Master.RiderLimit, "+
		"    Master.HeightsNeeded <> 0, "+
		"    Master.Deleted, "+
		"    MasterTourInfo.LongName, "+
		"    MasterTourInfo.Price, "+
		"    Riders.Count "+
		"FROM Master "+
		"LEFT JOIN MasterTourInfo ON Master.TourCode = MasterTourInfo.ShortCode "+
		"LEFT JOIN ("+
		"    SELECT TourID, SUM(Riders) AS Count "+
		"    FROM OrderItems "+
		"    GROUP BY TourID"+
		") AS Riders ON Master.TourID = Riders.TourID "+
		"WHERE Master.TourID = ?",
		tourID)
	err := row.Scan(&id, &code, &time, &confCode, &autoConfirm, &full, &cancelled, &riderLimit, &heightsNeeded, &deleted, &longName, &price, &numRiders)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	tourDetail := &TourDetail{
		Tour: Tour{
			ID:            id,
			Code:          code.String,
			Time:          time.Time,
			ConfCode:      confCode.String,
			AutoConfirm:   autoConfirm.Bool,
			Full:          full.Bool,
			Cancelled:     cancelled.Bool,
			HeightsNeeded: heightsNeeded.Bool,
			Deleted:       deleted.Bool,
		},
		LongName: longName.String,
		Price:    price.Float64,
	}
	if !riderLimit.Valid || riderLimit.Int64 == 0 {
		tourDetail.NumSpotsRemaining = maxRiders
	} else {
		tourDetail.NumSpotsRemaining = int(riderLimit.Int64) - int(numRiders.Int64)
	}
	return tourDetail, true, nil
}

func priceString(total uint64) string {
	return fmt.Sprintf("%d", total/100)
}

func heightsString(riders []Rider) string {
	var s []string
	for _, r := range riders {
		var h string
		switch {
		case r.Height <= 0:
			h = "??"
		case r.Height < 56:
			h = "<4'8"
		case r.Height > 78:
			h = ">6'6"
		default:
			h = fmt.Sprintf("%d'%d", r.Height/12, r.Height%12)
		}
		s = append(s, r.Gender+h)
	}
	return strings.Join(s, " ")
}

func (s *RemoteStore) prepareCreateOrder(tx *sql.Tx, tourID int32, numRiders int, riders []Rider, total uint64, name, email, mobile, hotel, misc string) (int32, error) {
	result, err := tx.Exec(
		"INSERT INTO OrderMain (CustName, CustEmail, Hotel, Mobile, DatePlaced, Message, Heights) VALUES (?, ?, ?, ?, ?, ?, ?)",
		name, email, hotel, mobile, time.Now(), misc, heightsString(riders))
	if err != nil {
		return 0, err
	}
	orderID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(
		"INSERT INTO OrderItems (OrderNum, TourID, Riders, Price, Method) VALUES (?, ?, ?, ?, ?)",
		orderID, tourID, numRiders, priceString(total), "STw")
	if err != nil {
		return 0, err
	}
	return int32(orderID), nil
}

func (s *RemoteStore) CreateOrder(tourID int32, numRiders int, riders []Rider, total uint64, name, email, mobile, hotel, misc string) (int32, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	orderID, err := s.prepareCreateOrder(tx, tourID, numRiders, riders, total, name, email, mobile, hotel, misc)
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
