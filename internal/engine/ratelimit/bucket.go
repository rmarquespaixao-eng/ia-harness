// Package ratelimit implementa o token bucket de requisições por provider
// (feature 019). A regra é pura e determinística: o tempo entra por parâmetro
// (vindo da porta Clock do harness), sem goroutines nem time.Now.
package ratelimit

import (
	"math"
	"time"
)

// Bucket é um token bucket de requisições: capacidade = burst, reposição =
// requestsPerMinute/60 por segundo. Zero/negativo desliga o limite.
type Bucket struct {
	rate      float64 // tokens por segundo
	capacity  float64
	tokens    float64
	last      time.Time
	started   bool
	limited   bool
	perMinute int
}

// New monta o bucket. requestsPerMinute ≤ 0 devolve um bucket desligado
// (Reserve sempre 0). Burst ≤ 0 cai para 1.
func New(requestsPerMinute, burst int) *Bucket {
	if requestsPerMinute <= 0 {
		return &Bucket{}
	}
	if burst <= 0 {
		burst = 1
	}
	return &Bucket{
		rate:      float64(requestsPerMinute) / 60.0,
		capacity:  float64(burst),
		tokens:    float64(burst),
		limited:   true,
		perMinute: requestsPerMinute,
	}
}

// Limited informa se há limite configurado.
func (b *Bucket) Limited() bool { return b != nil && b.limited }

// PerMinute devolve a taxa configurada (0 quando desligado).
func (b *Bucket) PerMinute() int {
	if b == nil {
		return 0
	}
	return b.perMinute
}

// Reserve consome um token e devolve a espera necessária a partir de now. Com
// tokens disponíveis devolve 0; caso contrário agenda o débito e devolve o
// tempo até o próximo token, de modo que chamadas concorrentes recebam esperas
// crescentes (cada uma reserva a sua vez). Relógio para trás nunca gera espera
// negativa.
func (b *Bucket) Reserve(now time.Time) time.Duration {
	if b == nil || !b.limited {
		return 0
	}
	if !b.started {
		b.started = true
		b.last = now
	}
	elapsed := now.Sub(b.last)
	if elapsed > 0 {
		b.tokens = math.Min(b.capacity, b.tokens+elapsed.Seconds()*b.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	deficit := 1 - b.tokens
	wait := deficit / b.rate
	b.tokens -= 1
	return time.Duration(wait * float64(time.Second))
}
