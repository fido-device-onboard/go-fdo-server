// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package rvinfo

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/protocol"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo-server/internal/utils"
)

func CreateRvInfo(useTLS bool, host string, port uint16) ([][]protocol.RvInstruction, error) {
	prot := protocol.RVProtHTTP
	if useTLS {
		prot = protocol.RVProtHTTPS
	}
	rvInfo := [][]protocol.RvInstruction{{{Variable: protocol.RVProtocol, Value: utils.MustMarshal(prot)}}}

	if host == "" {
		rvInfo[0] = append(rvInfo[0], protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: utils.MustMarshal(net.IP{127, 0, 0, 1})})
	} else if hostIP := net.ParseIP(host); hostIP.To4() != nil || hostIP.To16() != nil {
		rvInfo[0] = append(rvInfo[0], protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: utils.MustMarshal(hostIP)})
	} else {
		rvInfo[0] = append(rvInfo[0], protocol.RvInstruction{Variable: protocol.RVDns, Value: utils.MustMarshal(host)})
	}

	rvInfo[0] = append(rvInfo[0], protocol.RvInstruction{Variable: protocol.RVDevPort, Value: utils.MustMarshal(port)})

	return rvInfo, nil
}

func RetrieveRvInfo(rvInfo *[][]protocol.RvInstruction) error {
	rvData, err := db.FetchData("rvinfo")
	if err != nil {
		return fmt.Errorf("error fetching rvData after POST: %w", err)
	}

	parsedData, ok := rvData.Value.([]any)
	if !ok || len(parsedData) == 0 {
		return fmt.Errorf("error parsing rvData after POST: %v", rvData.Value)
	}

	for rvDirectiveIndex, rvDirective := range parsedData {
		rvMap, err := ParseRvMap(rvDirectiveIndex, rvDirective)
		if err != nil {
			slog.Debug("Error parsing item", "index", rvDirectiveIndex, "error", err)
			continue
		}
		if err := UpdateRvInfo(rvInfo, rvDirectiveIndex, rvMap); err != nil {
			return fmt.Errorf("error updating RVInfo: %w", err)
		}
	}
	return nil
}

func ParseRvMap(rvDirectiveIndex int, rvDirective any) (map[protocol.RvVar]any, error) {
	rvMap := make(map[protocol.RvVar]any)
	nestedItems, ok := rvDirective.([]any)
	if !ok {
		return nil, fmt.Errorf("error parsing item: %v", rvDirective)
	}
	for rvPairIndex, rvPair := range nestedItems {
		keyValue, ok := rvPair.([]any)
		if !ok || len(keyValue) < 1 {
			return nil, fmt.Errorf("error parsing pair %d in item: %v", rvPairIndex, rvPair)
		}
		key := keyValue[0]
		var value any = nil
		if len(keyValue) > 1 {
			value = keyValue[1]
		}

		keyRvVar, ok := key.(float64)
		if !ok {
			return nil, fmt.Errorf("error converting key to float64 in pair %d: %v", rvPairIndex, key)
		}

		rvMap[protocol.RvVar(keyRvVar)] = value
		utils.LogRvVar(rvDirectiveIndex, protocol.RvVar(keyRvVar), value)
	}
	return rvMap, nil
}

func UpdateRvInfo(rvInfo *[][]protocol.RvInstruction, index int, rvMap map[protocol.RvVar]any) error {
	var newRvInfo [][]protocol.RvInstruction

	if index > 0 {
		newRvInfo = make([][]protocol.RvInstruction, len(*rvInfo))
		copy(newRvInfo, *rvInfo)
	}

	for len(newRvInfo) <= index {
		newRvInfo = append(newRvInfo, make([]protocol.RvInstruction, 0))
	}

	if rvMap[protocol.RVProtocol] == nil {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVProtocol, Value: utils.MustMarshal(protocol.RVProtHTTP)})
	} else {
		isHttp := uint8(rvMap[protocol.RVProtocol].(float64))
		if isHttp == protocol.RVProtHTTP {
			newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVProtocol, Value: utils.MustMarshal(protocol.RVProtHTTP)})
		} else {
			newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVProtocol, Value: utils.MustMarshal(protocol.RVProtHTTPS)})
		}
	}

	if rvMap[protocol.RVDns] != nil {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVDns, Value: utils.MustMarshal(rvMap[protocol.RVDns].(string))})
	}

	if rvMap[protocol.RVIPAddress] == nil {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: utils.MustMarshal(net.IP{127, 0, 0, 1})})
	} else {
		host, ok := rvMap[protocol.RVIPAddress].(string)
		if !ok {
			return fmt.Errorf("error parsing rvData: Invalid RVIPAddress")
		}
		hostIP := net.ParseIP(host)
		if hostIP == nil {
			return fmt.Errorf("error parsing rvData: Invalid RVIPAddress: '%s'", host)
		}
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVIPAddress, Value: utils.MustMarshal(hostIP)})
	}

	if rvMap[protocol.RVDevPort] != nil {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVDevPort, Value: utils.MustMarshal(uint16(rvMap[protocol.RVDevPort].(float64)))})
	}

	if rvMap[protocol.RVOwnerPort] != nil {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVOwnerPort, Value: utils.MustMarshal(uint16(rvMap[protocol.RVOwnerPort].(float64)))})
	}

	if rvMap[protocol.RVDelaysec] != nil {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVDelaysec, Value: utils.MustMarshal(uint16(rvMap[protocol.RVDelaysec].(float64)))})
	}

	if _, ok := rvMap[protocol.RVBypass]; ok {
		newRvInfo[index] = append(newRvInfo[index], protocol.RvInstruction{Variable: protocol.RVBypass})
	}

	*rvInfo = newRvInfo

	return nil
}

func FetchRvInfo() ([][]protocol.RvInstruction, error) {
	var rvInfo [][]protocol.RvInstruction

	if exists, err := db.CheckDataExists("rvinfo"); err != nil {
		slog.Debug("Error checking rvData existence", "error", err)
		return nil, err
	} else if exists {
		if err := RetrieveRvInfo(&rvInfo); err != nil {
			slog.Debug("Error retrieving RVInfo", "error", err)
			return nil, err
		}
	} else if !exists {
		return nil, err
	}
	return rvInfo, nil
}

func GetRVIPAddress(rvInfo [][]protocol.RvInstruction) (string, string, error) {
	var ipAddress, dnsAddress string
	var port uint16
	var proto uint8

	for _, instructions := range rvInfo {
		for _, instruction := range instructions {
			var err error
			switch instruction.Variable {
			case protocol.RVIPAddress:
				var ip []byte
				err = cbor.Unmarshal(instruction.Value, &ip)
				if err == nil {
					ipAddress = net.IP(ip).String()
				}
			case protocol.RVDns:
				err = cbor.Unmarshal(instruction.Value, &dnsAddress)
			case protocol.RVDevPort, protocol.RVOwnerPort:
				err = cbor.Unmarshal(instruction.Value, &port)
			case protocol.RVProtocol:
				var prot uint8
				err = cbor.Unmarshal(instruction.Value, &prot)
				proto = uint8(prot)
			}
			if err != nil {
				return "", "", fmt.Errorf("invalid format for %v: %v", instruction.Variable, err)
			}
		}
	}

	if ipAddress == "" && dnsAddress == "" {
		return "", "", fmt.Errorf("no IP address or DNS address found")
	}

	scheme := map[uint8]string{
		protocol.RVProtHTTP:  "http",
		protocol.RVProtHTTPS: "https",
	}[proto]

	if scheme == "" {
		return "", "", fmt.Errorf("unsupported protocol")
	}

	var url1, url2 url.URL

	if ipAddress != "" && dnsAddress != "" {
		url1 = url.URL{
			Scheme: scheme,
			Host:   net.JoinHostPort(ipAddress, strconv.Itoa(int(port))),
		}
		url2 = url.URL{
			Scheme: scheme,
			Host:   net.JoinHostPort(dnsAddress, strconv.Itoa(int(port))),
		}
		return url1.String(), url2.String(), nil
	}

	host := ipAddress
	if host == "" {
		host = dnsAddress
	}

	url1 = url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, strconv.Itoa(int(port))),
	}

	return url1.String(), "", nil
}

func GetRvInfoFromVoucher(voucherData []byte) ([][]protocol.RvInstruction, error) {
	var voucher fdo.Voucher
	if err := cbor.Unmarshal(voucherData, &voucher); err != nil {
		return nil, fmt.Errorf("failed to unmarshal voucher: %v", err)
	}

	return voucher.Header.Val.RvInfo, nil
}

func HasRVBypass(rvInfo [][]protocol.RvInstruction) bool {
	for _, instructions := range rvInfo {
		for _, instruction := range instructions {
			if instruction.Variable == protocol.RVBypass {
				return true
			}
		}
	}
	return false
}
