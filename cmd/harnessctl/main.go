// Command harnessctl é a CLI de desenvolvimento/smoke do núcleo do harness
// (T080, D-13): monta a Config por flags, roda um turno com um Handler que
// imprime os eventos no stdout e retoma pelo terminal as confirmações exigidas
// pela política.
//
// NÃO é serviço e não tem superfície estável: a constitution §9 o lista
// explicitamente fora de escopo como produto — sem API/rede própria, sem
// persistência durável (sessões só em memória) e sem garantia de compatibilidade.
// O host real embute a biblioteca in-process (contracts/library-api.md).
//
// Exemplo:
//
//	harnessctl -prompt "qual o saldo deste mês?" \
//	  -model openai/gpt-4o-mini -credential-ref env:OPENROUTER_API_KEY \
//	  -provider-url https://openrouter.ai/api/v1 \
//	  -mcp-url https://homolog-api-financeiro.homelab-cloud.com/mcp \
//	  -mcp-credential-ref env:FINANCEIRO_MCP_KEY \
//	  -allow-tools 'financeiro.*' -confirm-tools 'financeiro.pagar_fatura'
//
// O default da política é deny: sem -allow-tools nenhuma tool executa. Nenhum
// valor de credencial é logado; referências env:/file: são resolvidas sob
// demanda, nunca antes da chamada (FR-004).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	auditlog "rmarquespaixao/ia-harness/adapters/audit/log"
	"rmarquespaixao/ia-harness/adapters/mcpclient"
	"rmarquespaixao/ia-harness/adapters/provider/anthropic"
	"rmarquespaixao/ia-harness/adapters/provider/openai"
	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
)

const (
	// providerKindOpenAI e providerKindAnthropic são os adaptadores aceitos.
	providerKindOpenAI    = "openai"
	providerKindAnthropic = "anthropic"
	// mcpNamespace é o namespace das tools publicadas pelo servidor MCP.
	mcpNamespace = "mcp"
	// defaultModel é o alias/nome usado quando -model é omitido.
	defaultModel = "gpt-4o-mini"
	// maxConfirmations limita as pausas de confirmação de um único comando.
	maxConfirmations = 16
)

// options reúne as flags da CLI já validadas.
type options struct {
	prompt           string
	sessionID        string
	userID           string
	agentID          string
	model            string
	providerURL      string
	providerKind     string
	credentialRef    string
	mcpURL           string
	mcpCredentialRef string
	pricingJSON      string
	allowTools       []string
	confirmTools     []string
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "harnessctl:", err)
		os.Exit(1)
	}
}

// run monta o harness, executa o turno e resolve as confirmações pendentes no
// terminal. Separado de main para manter o fluxo testável sem tocar o processo.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	opts, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	cfg, err := buildConfig(opts, logger)
	if err != nil {
		return err
	}
	h, err := harness.New(cfg)
	if err != nil {
		return fmt.Errorf("montar harness: %w", err)
	}
	defer func() { _ = h.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler := &stdoutHandler{out: stdout}
	result, err := h.Run(ctx, harness.RunRequest{
		SessionID: opts.sessionID,
		UserID:    opts.userID,
		AgentID:   opts.agentID,
		Model:     opts.model,
		Input:     []harness.Part{{Kind: harness.PartText, Text: opts.prompt}},
	}, handler)
	if err != nil {
		return fmt.Errorf("turno: %w", err)
	}

	scanner := bufio.NewScanner(stdin)
	for pending := 0; result.State == harness.SessionAwaitingConfirmation; pending++ {
		if result.Pending == nil {
			return errors.New("sessão aguardando confirmação sem pendência registrada")
		}
		if pending >= maxConfirmations {
			return fmt.Errorf("mais de %d confirmações no mesmo comando; abortando", maxConfirmations)
		}
		printPending(stdout, result.Pending)
		decision, err := readDecision(scanner, stdout)
		if err != nil {
			return err
		}
		result, err = h.ResolveConfirmation(ctx, result.SessionID, result.Pending.CallID, decision, handler)
		if err != nil {
			return fmt.Errorf("resolver confirmação: %w", err)
		}
	}

	printResult(stdout, result)
	return nil
}

// parseOptions interpreta e valida as flags; -prompt é obrigatória.
func parseOptions(args []string, stderr io.Writer) (options, error) {
	fs := flag.NewFlagSet("harnessctl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		prompt        = fs.String("prompt", "", "pergunta do turno (obrigatória)")
		sessionID     = fs.String("session", "", "id de sessão existente; vazio cria uma nova")
		userID        = fs.String("user", "dev", "id do usuário dono da sessão")
		agentID       = fs.String("agent", "harnessctl", "id do agente usado na política")
		model         = fs.String("model", defaultModel, "alias e nome do modelo enviado ao provedor")
		providerURL   = fs.String("provider-url", "", "base URL do provedor (vazio usa o default do adaptador)")
		providerKind  = fs.String("provider-kind", providerKindOpenAI, "adaptador do provedor: openai|anthropic")
		credentialRef = fs.String("credential-ref", "", "credencial do provedor (env:NOME ou file:/caminho)")
		mcpURL        = fs.String("mcp-url", "", "endpoint Streamable HTTP do MCP (vazio = sem tools)")
		mcpCredRef    = fs.String("mcp-credential-ref", "", "credencial do MCP (env:NOME ou file:/caminho)")
		pricingJSON   = fs.String("pricing-json", "", "JSON de harness.Pricing (vazio usa o default do núcleo)")
		allowTools    = fs.String("allow-tools", "", "globs CSV de tools permitidas (ex.: 'financeiro.*'); vazio = deny")
		confirmTools  = fs.String("confirm-tools", "", "globs CSV de tools que exigem confirmação humana")
	)
	fs.Usage = func() {
		fmt.Fprint(stderr, "uso: harnessctl -prompt \"...\" [flags]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	opts := options{
		prompt:           strings.TrimSpace(*prompt),
		sessionID:        strings.TrimSpace(*sessionID),
		userID:           strings.TrimSpace(*userID),
		agentID:          strings.TrimSpace(*agentID),
		model:            strings.TrimSpace(*model),
		providerURL:      strings.TrimSpace(*providerURL),
		providerKind:     strings.ToLower(strings.TrimSpace(*providerKind)),
		credentialRef:    strings.TrimSpace(*credentialRef),
		mcpURL:           strings.TrimSpace(*mcpURL),
		mcpCredentialRef: strings.TrimSpace(*mcpCredRef),
		pricingJSON:      strings.TrimSpace(*pricingJSON),
		allowTools:       splitGlobs(*allowTools),
		confirmTools:     splitGlobs(*confirmTools),
	}
	switch {
	case opts.prompt == "":
		return options{}, errors.New("flag -prompt é obrigatória")
	case opts.userID == "":
		return options{}, errors.New("flag -user não pode ser vazia")
	case opts.agentID == "":
		return options{}, errors.New("flag -agent não pode ser vazia")
	case opts.model == "":
		return options{}, errors.New("flag -model não pode ser vazia")
	case opts.providerKind != providerKindOpenAI && opts.providerKind != providerKindAnthropic:
		return options{}, fmt.Errorf("flag -provider-kind %q inválida (use openai ou anthropic)", opts.providerKind)
	case len(opts.confirmTools) > 0 && len(opts.allowTools) == 0:
		fmt.Fprintln(stderr, "aviso: -confirm-tools sem -allow-tools não tem efeito (política default deny)")
	}
	return opts, nil
}

// splitGlobs separa o CSV de globs, descartando itens vazios.
func splitGlobs(csv string) []string {
	var out []string
	for _, item := range strings.Split(csv, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// buildConfig monta a Config por injeção explícita (DI — ADR 0006): adaptadores
// instanciados aqui, núcleo só valida e compõe.
func buildConfig(opts options, logger *slog.Logger) (harness.Config, error) {
	credentials := credentialProvider{}
	provider, err := newProvider(opts, credentials, logger)
	if err != nil {
		return harness.Config{}, err
	}
	pricing, err := parsePricing(opts.pricingJSON)
	if err != nil {
		return harness.Config{}, err
	}

	var tools []harness.ToolSource
	if opts.mcpURL != "" {
		tools = append(tools, mcpclient.New(mcpclient.Config{
			Name:          mcpNamespace,
			Endpoint:      opts.mcpURL,
			CredentialRef: opts.mcpCredentialRef,
		}, mcpclient.Deps{Credentials: credentials, Logger: logger}))
	}

	return harness.Config{
		Providers: map[string]harness.Provider{opts.providerKind: provider},
		Models: map[string]harness.ModelProfile{
			opts.model: {
				Provider: opts.providerKind,
				Model:    opts.model,
				Capabilities: harness.Capabilities{
					ToolCalling: true,
					Streaming:   true,
				},
			},
		},
		Tools:        tools,
		Policy:       buildPolicy(opts),
		Pricing:      pricing,
		DefaultModel: opts.model,
		Credentials:  credentials,
		Sessions:     memory.New(),
		Logger:       logger,
		Audit:        auditlog.New(slog.Default()),
	}, nil
}

// newProvider instancia o adaptador escolhido por -provider-kind.
func newProvider(opts options, credentials harness.CredentialProvider, logger *slog.Logger) (harness.Provider, error) {
	switch opts.providerKind {
	case providerKindOpenAI:
		return openai.New(openai.Config{
			BaseURL:       opts.providerURL,
			CredentialRef: opts.credentialRef,
			DefaultModel:  opts.model,
		}, openai.Deps{Credentials: credentials, Logger: logger}), nil
	case providerKindAnthropic:
		return anthropic.New(anthropic.Config{
			BaseURL:       opts.providerURL,
			CredentialRef: opts.credentialRef,
			DefaultModel:  opts.model,
		}, anthropic.Deps{Credentials: credentials, Logger: logger}), nil
	default:
		return nil, fmt.Errorf("provider-kind %q inválida", opts.providerKind)
	}
}

// buildPolicy parte do default deny do núcleo; com -allow-tools declara o agente
// em modo allow com a allowlist e as confirmações informadas.
func buildPolicy(opts options) harness.PolicyConfig {
	policy := harness.PolicyConfig{Default: harness.PolicyDeny}
	if len(opts.allowTools) == 0 {
		return policy
	}
	policy.Agents = map[string]harness.AgentPolicy{
		opts.agentID: {
			Mode:         harness.PolicyAllow,
			AllowTools:   opts.allowTools,
			ConfirmTools: opts.confirmTools,
		},
	}
	return policy
}

// parsePricing decodifica a premissa de custo do flag; vazio mantém os defaults.
func parsePricing(raw string) (harness.Pricing, error) {
	if raw == "" {
		return harness.Pricing{}, nil
	}
	var pricing harness.Pricing
	if err := json.Unmarshal([]byte(raw), &pricing); err != nil {
		return harness.Pricing{}, fmt.Errorf("flag -pricing-json inválida: %w", err)
	}
	return pricing, nil
}

// credentialProvider resolve "env:NOME" e "file:/caminho" sob demanda; o valor
// existe apenas no ponto de uso e nunca é logado.
type credentialProvider struct{}

// Resolve implementa harness.CredentialProvider.
func (credentialProvider) Resolve(ctx context.Context, ref string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ref = strings.TrimSpace(ref)
	switch {
	case strings.HasPrefix(ref, "env:"):
		name := strings.TrimSpace(strings.TrimPrefix(ref, "env:"))
		if name == "" {
			return "", fmt.Errorf("credential: referência %q sem o nome da variável", ref)
		}
		value, ok := os.LookupEnv(name)
		if !ok || value == "" {
			return "", fmt.Errorf("credential: variável de ambiente %q não definida", name)
		}
		return value, nil
	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "file:"))
		if path == "" {
			return "", fmt.Errorf("credential: referência %q sem o caminho do arquivo", ref)
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("credential: arquivo %q inacessível: %w", path, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("credential: %q é um diretório, não um arquivo de credencial", path)
		}
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			return "", fmt.Errorf("credential: arquivo %q com permissões %#o; exija 0600 (chmod 600)", path, mode)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("credential: ler arquivo %q: %w", path, err)
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", fmt.Errorf("credential: arquivo %q vazio", path)
		}
		return value, nil
	default:
		return "", fmt.Errorf("credential: referência %q inválida (use env:NOME ou file:/caminho)", ref)
	}
}

// stdoutHandler imprime os eventos do turno no stdout. Confirmation devolve erro
// para o núcleo persistir a pendência (State=awaiting_confirmation); a decisão é
// colhida no terminal e aplicada por ResolveConfirmation — nunca aqui.
type stdoutHandler struct {
	harness.NopHandler
	out io.Writer
}

func (h *stdoutHandler) ReasoningDelta(_ context.Context, ev harness.ReasoningDelta) {
	fmt.Fprintf(h.out, "[reasoning] %s", ev.Text)
}

func (h *stdoutHandler) ToolCallDelta(_ context.Context, ev harness.ToolCallArgsDelta) {
	fmt.Fprintf(h.out, "[tool_args] %s %s", ev.Tool, ev.ArgsFragment)
}

func (h *stdoutHandler) TextDelta(_ context.Context, ev harness.TextDelta) {
	fmt.Fprint(h.out, ev.Text)
}

func (h *stdoutHandler) ToolCall(_ context.Context, ev harness.ToolCallEvent) {
	fmt.Fprintf(h.out, "\n[tool_call] %s call_id=%s args_bytes=%d args=%s\n", ev.Tool, ev.CallID, ev.ArgsBytes, ev.ArgsRedacted)
}

func (h *stdoutHandler) ToolResult(_ context.Context, ev harness.ToolResultEvent) {
	fmt.Fprintf(h.out, "\n[tool_result] %s call_id=%s status=%s is_error=%t denied=%t truncated=%t latency_ms=%d summary=%s\n",
		ev.Tool, ev.CallID, ev.Status, ev.IsError, ev.Denied, ev.Truncated, ev.LatencyMS, ev.ResultSummary)
}

func (h *stdoutHandler) Progress(_ context.Context, ev harness.ProgressEvent) {
	fmt.Fprintf(h.out, "\n[progress] %s call_id=%s %.2f/%.2f %s\n", ev.Tool, ev.CallID, ev.Progress, ev.Total, ev.Message)
}

func (h *stdoutHandler) Usage(_ context.Context, ev harness.UsageEvent) {
	fmt.Fprintf(h.out, "\n[usage] provider=%s model=%s input_tokens=%d output_tokens=%d estimated=%t cost_micros=%d currency=%s\n",
		ev.Provider, ev.Model, ev.InputTokens, ev.OutputTokens, ev.Estimated, ev.CostMicros, ev.Currency)
}

func (h *stdoutHandler) Error(_ context.Context, ev harness.ErrorEvent) {
	fmt.Fprintf(h.out, "\n[error] scope=%s tool=%s retryable=%t message=%s\n", ev.Scope, ev.Tool, ev.Retryable, ev.Message)
}

func (h *stdoutHandler) Confirmation(_ context.Context, ev harness.ConfirmationEvent) (harness.Decision, error) {
	return harness.Decision{}, fmt.Errorf("confirmação de %s (%s) pendente no terminal", ev.Tool, ev.CallID)
}

// printPending mostra a confirmação registrada na sessão.
func printPending(out io.Writer, pending *harness.PendingConfirmation) {
	fmt.Fprintln(out, "\n--- confirmação pendente ---")
	fmt.Fprintf(out, "tool=%s call_id=%s\n", pending.ToolName, pending.CallID)
	fmt.Fprintf(out, "motivo=%s\n", pending.Reason)
	fmt.Fprintf(out, "args=%s\n", pending.ArgsRedacted)
}

// readDecision lê "approve"/"deny" do terminal até obter uma resposta válida.
func readDecision(scanner *bufio.Scanner, out io.Writer) (harness.Decision, error) {
	for {
		fmt.Fprint(out, "decisão [approve/deny]> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return harness.Decision{}, fmt.Errorf("ler decisão do terminal: %w", err)
			}
			return harness.Decision{}, errors.New("stdin encerrado sem decisão (approve/deny)")
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "approve", "a", "y", "yes":
			return harness.Decision{Approve: true}, nil
		case "deny", "d", "n", "no":
			return harness.Decision{Approve: false, Reason: "negado pelo operador no terminal"}, nil
		default:
			fmt.Fprintln(out, "resposta inválida: use approve ou deny")
		}
	}
}

// printResult resume o TurnResult ao final do comando.
func printResult(out io.Writer, result harness.TurnResult) {
	fmt.Fprintln(out, "\n--- turn_result ---")
	fmt.Fprintf(out, "session_id=%s state=%s stop_reason=%s model=%s\n",
		result.SessionID, result.State, result.StopReason, result.Model)
	if text := partsText(result.Output); text != "" {
		fmt.Fprintf(out, "output=%s\n", text)
	}
	fmt.Fprintf(out, "usage input_tokens=%d output_tokens=%d estimated=%t cost_micros=%d currency=%s\n",
		result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.Estimated, result.Usage.CostMicros, result.Usage.Currency)
	for _, exec := range result.ToolCalls {
		fmt.Fprintf(out, "tool call_id=%s tool=%s status=%s latency_ms=%d\n",
			exec.CallID, exec.Tool, exec.Status, exec.LatencyMS)
	}
}

// partsText concatena as partes de texto da saída final.
func partsText(parts []harness.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Kind == harness.PartText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}
