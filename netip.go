package main

import "net/netip"

func netipParse(raw string) (string, error) {
	prefix, err := netip.ParsePrefix(raw)
	if err != nil {
		return "", err
	}
	return prefix.Masked().String(), nil
}
