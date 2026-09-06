// Package strutil fornece helpers de manipulacao de strings.
package strutil

import (
	"strconv"
	"strings"
)

// OnlyDigits retorna s com todos os caracteres nao numericos removidos.
func OnlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// MustAtoi converte s em int ignorando erros (0 em caso de falha).
func MustAtoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
