// Package env centraliza a leitura de variaveis de ambiente da biblioteca,
// com valores padrao e validacao.
package env

import (
	"os"
	"strconv"
)

// String retorna o valor da variavel de ambiente key, ou fallback quando vazia.
func String(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// StringOr retorna option quando nao vazio, senao le a variavel key (fallback
// se vazia). Permite sobrepor configuracao por ambiente a partir do codigo.
func StringOr(option, key, fallback string) string {
	if option != "" {
		return option
	}
	return String(key, fallback)
}

// IntOr retorna option quando positivo, senao le a variavel key (fallback se
// vazia/invalida). Permite sobrepor configuracao por ambiente a partir do codigo.
func IntOr(option int, key string, fallback int) int {
	if option > 0 {
		return option
	}
	return Int(key, fallback)
}

// Int retorna o inteiro nao negativo da variavel key, ou fallback quando vazia,
// invalida ou negativa.
func Int(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return fallback
}

// Int32 retorna o inteiro positivo de 32bits da variavel key, ou fallback
// quando vazia, invalida ou nao positiva.
func Int32(key string, fallback int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			return int32(n)
		}
	}
	return fallback
}
