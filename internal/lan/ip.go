// Package lan implementa el Modo LAN (M7): detección de la IP local, listener
// HTTP reiniciable (bind 127.0.0.1 <-> 0.0.0.0), y el guardián que decide qué
// puede hacer un cliente que NO es el propio PC (PIN, rate-limit y lista de
// rutas permitidas).
package lan

import (
	"net"
	"regexp"
	"sort"
)

// Candidate es una IPv4 local que un móvil podría usar para llegar a GALA.
type Candidate struct {
	IP        string `json:"ip"`
	Interface string `json:"interface"`
	// Virtual marca interfaces que parecen de VPN, máquina virtual o
	// contenedor: solo se usan como último recurso.
	Virtual bool `json:"virtual"`
}

// ifaceAddr es una dirección de una interfaz; separa la enumeración del
// sistema (no testeable) de la elección (sí testeable).
type ifaceAddr struct {
	Name  string
	Flags net.Flags
	IP    net.IP
}

var virtualName = regexp.MustCompile(`(?i)vethernet|virtualbox|vmware|vbox|docker|wsl|hyper-v|tailscale|zerotier|hamachi|wireguard|npcap|loopback|pseudo|bridge|virbr|veth|utun|\btun|\btap|vpn|^br-|^wg\d`)

// Candidates devuelve las IPv4 candidatas de esta máquina, la mejor primero.
// Vacío si no hay ninguna red activa.
func Candidates() []Candidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var all []ifaceAddr
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				all = append(all, ifaceAddr{Name: ifc.Name, Flags: ifc.Flags, IP: ipn.IP})
			}
		}
	}
	return rank(all)
}

// rank filtra y ordena: interfaces físicas con IP privada primero
// (192.168/16, luego 10/8, luego 172.16/12), luego públicas, y al final las
// que parecen virtuales. Excluye loopback, link-local, IPv6 y interfaces caídas.
func rank(in []ifaceAddr) []Candidate {
	type scored struct {
		c     Candidate
		score int
	}
	var out []scored
	for _, a := range in {
		if a.Flags&net.FlagUp == 0 || a.Flags&net.FlagLoopback != 0 {
			continue
		}
		ip4 := a.IP.To4()
		if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
			continue
		}
		virtual := virtualName.MatchString(a.Name)
		score := 5
		switch {
		case ip4[0] == 192 && ip4[1] == 168:
			score = 0
		case ip4[0] == 10:
			score = 1
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			score = 2
		}
		if virtual {
			score += 100
		}
		out = append(out, scored{Candidate{IP: ip4.String(), Interface: a.Name, Virtual: virtual}, score})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score < out[j].score })
	res := make([]Candidate, len(out))
	for i, s := range out {
		res[i] = s.c
	}
	return res
}
