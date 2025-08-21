package ownerinfo

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/fido-device-onboard/go-fdo-server/internal/db"
	"github.com/fido-device-onboard/go-fdo/protocol"
)

func RetrieveOwnerInfo() ([]protocol.RvTO2Addr, error) {
	ownerData, err := db.FetchData("owner_info")
	if err != nil {
		return nil, fmt.Errorf("error fetching rvData after POST: %w", err)
	}

	parsedData, ok := ownerData.Value.([]interface{})
	if !ok || len(parsedData) == 0 {
		return nil, fmt.Errorf("error parsing ownerData after POST: %v", ownerData.Value)
	}

	rvTO2Addrs, err := parseRvTO2Addr(parsedData)
	if err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}
	return rvTO2Addrs, nil
}

func FetchOwnerInfo() ([]protocol.RvTO2Addr, error) {
	var ownerInfo []protocol.RvTO2Addr

	if exists, err := db.CheckDataExists("owner_info"); err != nil {
		slog.Debug("Error checking ownerInfo existence", "error", err)
		return nil, err
	} else if exists {
		if ownerInfo, err = RetrieveOwnerInfo(); err != nil {
			slog.Debug("Error retrieving ownerInfo", "error", err)
			return nil, err
		}
	} else if !exists {
		return nil, err
	}
	return ownerInfo, nil
}

func parseRvTO2Addr(ownerData []interface{}) ([]protocol.RvTO2Addr, error) {
	var rvTO2Addrs []protocol.RvTO2Addr
	for _, item := range ownerData {
		itemSlice, ok := item.([]interface{})
		if !ok || len(itemSlice) != 4 {
			return nil, fmt.Errorf("invalid data format")
		}

		var ip *net.IP
		if itemSlice[0] != nil {
			ipStr, ok := itemSlice[0].(string)
			if !ok {
				return nil, fmt.Errorf("invalid IP address format")
			}
			parsedIP := net.ParseIP(ipStr)
			ip = &parsedIP
		}

		var dnsStr *string
		if itemSlice[1] != nil {
			dns, ok := itemSlice[1].(string)
			if !ok {
				return nil, fmt.Errorf("invalid DNS address format")
			}
			dnsStr = &dns
		}

		port, ok := itemSlice[2].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid port format")
		}

		proto, ok := itemSlice[3].(float64)
		if !ok {
			return nil, fmt.Errorf("invalid transport protocol format")
		}

		rvTO2Addr := protocol.RvTO2Addr{
			IPAddress:         ip,
			DNSAddress:        dnsStr,
			Port:              uint16(port),
			TransportProtocol: protocol.TransportProtocol(proto),
		}
		rvTO2Addrs = append(rvTO2Addrs, rvTO2Addr)
	}

	return rvTO2Addrs, nil
}

func CreateRvTO2Addr(host string, port uint16, useTLS bool) error {
	var rvTO2Addrs [][]interface{}

	ip := net.ParseIP(host)

	var proto protocol.TransportProtocol
	if useTLS {
		proto = protocol.HTTPSTransport
	} else {
		proto = protocol.HTTPTransport
	}

	if ip != nil {
		rvTO2Addrs = append(rvTO2Addrs, []interface{}{
			ip.String(),
			nil,
			port,
			proto,
		})
	} else {
		rvTO2Addrs = append(rvTO2Addrs, []interface{}{
			nil,
			host,
			port,
			proto,
		})
	}

	err := StoreRvTO2Addrs(rvTO2Addrs)
	if err != nil {
		return fmt.Errorf("failed to store rvTO2Addrs: %v", err)
	}

	return nil
}

func StoreRvTO2Addrs(rvTO2Addrs [][]interface{}) error {
	var ownerData db.Data
	ownerData.Value = rvTO2Addrs

	if exists, err := db.CheckDataExists("owner_info"); err != nil {
		slog.Debug("Error checking ownerInfo existence", "error", err)
		return err
	} else if !exists {
		if err := db.InsertData(ownerData, "owner_info"); err != nil {
			slog.Debug("Error inserting ownerData", "error", err)
			return err
		}
	}

	return nil
}
