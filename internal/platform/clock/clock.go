// Package clock é a abstração de tempo do harness: única ocorrência de
// time.Now() na árvore, permitindo relógio falso nos testes.
package clock

import "time"

// System é o relógio baseado no relógio do sistema operacional.
type System struct{}

// Now devolve o instante atual do sistema.
func (System) Now() time.Time {
	return time.Now()
}
