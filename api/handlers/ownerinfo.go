// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"errors"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"gorm.io/gorm"
)

func OwnerInfoHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getOwnerInfo(w, r)
	case http.MethodPost:
		createOwnerInfo(w, r)
	case http.MethodPut:
		updateOwnerInfo(w, r)
	default:
		WriteErrorResponse(w, r, http.StatusMethodNotAllowed, "Method not allowed", "HTTP method "+r.Method+" is not supported for this endpoint", "Method not allowed")
	}
}

func getOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfoJSON, err := db.FetchOwnerInfoJSON()
	if err != nil {
		handleOwnerInfoError(w, r, err, "fetching")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfoJSON)
}

func handleOwnerInfoError(w http.ResponseWriter, r *http.Request, err error, operation string) bool {

	if HandleDBError(w, r, "ownerInfo", err) {
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		WriteErrorResponse(w, r, http.StatusNotFound, "ownerInfo does not exist", "No ownerInfo found to update", "ownerInfo does not exist")
		return true
	}
	if errors.Is(err, db.ErrInvalidOwnerInfo) {
		WriteErrorResponse(w, r, http.StatusBadRequest, "Invalid ownerInfo", err.Error(), "Invalid ownerInfo")
		return true
	}
	errorMsg := "Error " + operation + " ownerInfo"
	WriteErrorResponse(w, r, http.StatusInternalServerError, errorMsg, err.Error(), errorMsg)
	return true
}

func createOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	if err := db.InsertOwnerInfo(ownerInfo); err != nil {
		handleOwnerInfoError(w, r, err, "inserting")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	w.Write(ownerInfo)
}

func updateOwnerInfo(w http.ResponseWriter, r *http.Request) {
	ownerInfo, ok := ReadRequestBody(w, r)
	if !ok {
		return
	}

	if err := db.UpdateOwnerInfo(ownerInfo); err != nil {
		handleOwnerInfoError(w, r, err, "updating")
		return
	}

	w.Header().Set("Content-Type", ContentTypeJSON)
	w.Write(ownerInfo)
}
