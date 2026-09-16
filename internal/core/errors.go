package core

// ConfigError indica configuração inválida (falha rápida em New).
type ConfigError struct {
	Code    string
	Message string
	Err     error
}

func (e *ConfigError) Error() string { return namedErrorText(e.Code, e.Message, e.Err) }

// Unwrap devolve a causa para errors.Is/errors.As.
func (e *ConfigError) Unwrap() error { return e.Err }

// PolicyError indica execução negada ou inválida segundo a política.
type PolicyError struct {
	Code    string
	Message string
	Err     error
}

func (e *PolicyError) Error() string { return namedErrorText(e.Code, e.Message, e.Err) }

// Unwrap devolve a causa para errors.Is/errors.As.
func (e *PolicyError) Unwrap() error { return e.Err }

// ToolError indica falha ao executar ou validar uma tool.
type ToolError struct {
	Code    string
	Message string
	Err     error
}

func (e *ToolError) Error() string { return namedErrorText(e.Code, e.Message, e.Err) }

// Unwrap devolve a causa para errors.Is/errors.As.
func (e *ToolError) Unwrap() error { return e.Err }

// ProviderError indica falha do adaptador de modelo.
type ProviderError struct {
	Code    string
	Message string
	Err     error
}

func (e *ProviderError) Error() string { return namedErrorText(e.Code, e.Message, e.Err) }

// Unwrap devolve a causa para errors.Is/errors.As.
func (e *ProviderError) Unwrap() error { return e.Err }

// OutputError indica que a saída final não corresponde ao schema pedido no
// turno (structured output — feature 009).
type OutputError struct {
	Code    string
	Message string
	Err     error
}

func (e *OutputError) Error() string { return namedErrorText(e.Code, e.Message, e.Err) }

// Unwrap devolve a causa para errors.Is/errors.As.
func (e *OutputError) Unwrap() error { return e.Err }

// namedErrorText monta a mensagem estável "código: mensagem: causa".
func namedErrorText(code, message string, err error) string {
	out := code
	if message != "" {
		if out != "" {
			out += ": "
		}
		out += message
	}
	if err != nil {
		if out != "" {
			out += ": "
		}
		out += err.Error()
	}
	return out
}
