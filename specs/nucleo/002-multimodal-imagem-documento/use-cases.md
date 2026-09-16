# Casos de uso — 002 Multimodal (imagem/documento)

**Formato**: caso de uso *fully-dressed* (Cockburn) + critérios de aceite em Gherkin.
**Atores**: Usuário (final, do host) · Host (aplicação que embute o harness) · Harness (núcleo) · Provedor de modelo · (apoio) Adaptador OpenAI-compatible / Anthropic.
**Rastreabilidade**: cada CU mapeia para FR-MM-* do `spec.md`; `tasks.md` cita os CU.

---

## CU-MM-1 — Enviar imagem para o modelo

- **Identificador:** CU-MM-1
- **Escopo:** harness (núcleo + adaptador de provedor)
- **Nível:** objetivo do usuário
- **Ator primário:** Usuário
- **Atores de apoio:** Host, Harness, Provedor
- **Partes interessadas e interesses:** Usuário (resposta sobre a imagem); Operador (custo/segurança); Host (não receber erro cru).
- **Pré-condições:** sessão ativa; perfil de modelo corrente com `capabilities.vision=true`; imagem em MIME da allowlist e dentro do teto.
- **Gatilho:** Host chama `Run` com uma mensagem contendo `Part{kind:image, media:{mime, bytes|url}}`.
- **Garantia de sucesso:** o provedor recebe a imagem no formato nativo e o turno conclui com resposta textual.
- **Garantia mínima:** nenhuma mídia vaza em log/auditoria; em falha, a sessão permanece retomável e o erro é nomeado.
- **Mapeamento técnico:** `Provider.Chat` → `openai` `content[].image_url` / `anthropic` `content_block{type:image}`.
- **Fluxo principal de sucesso:**
  1. Host monta a mensagem com a Part de imagem e chama `Run`.
  2. Harness valida MIME (allowlist), tamanho (teto) e fonte única (bytes xor url).
  3. Harness confere `Capabilities.Vision` do perfil; aprovado.
  4. Loop monta `ChatRequest` com a Part de imagem.
  5. Adaptador mapeia para o formato nativo e envia ao provedor.
  6. Provedor responde; eventos de texto chegam ao Host; turno conclui e a sessão é salva.
- **Fluxos alternativos e de exceção:**
  - **2a. MIME fora da allowlist:** erro nomeado; sem I/O de modelo.
  - **2b. Tamanho acima do teto:** erro nomeado; sem I/O.
  - **3a. `vision=false`:** falha explícita (FR-MM-003), provedor não é chamado.
  - **5a. Provedor rejeita a imagem:** erro classificado e devolvido; fallback só para perfil com visão.
- **Regras de negócio:** allowlist de MIME; teto de bytes; capacidade por perfil.
- **Requisitos especiais (NFR):** redação de bytes/URL (FR-MM-008); sem rede real em teste (FR-MM-012).
- **Dados e variações:** MIME (png/jpeg/webp/gif), fonte (bytes/url), tamanho.
- **Questões em aberto:** L-MM-1 (teto default), L-MM-4 (descarte de mídia antiga).
- **Critérios de aceite (Gherkin):**
  ```gherkin
  Cenário: imagem suportada chega ao provedor
    Dado um perfil com capabilities.vision=true
    E uma imagem PNG dentro do teto
    Quando o Host executa o turno com a imagem
    Então o adaptador envia um bloco de imagem no formato nativo
    E o turno conclui com StopReason=completed

  Cenário: perfil sem visão falha sem chamar o provedor
    Dado um perfil com capabilities.vision=false
    E uma imagem anexada
    Quando o Host executa o turno
    Então o harness devolve erro nomeado
    E nenhuma chamada ao provedor é feita

  Cenário: MIME fora da allowlist
    Dado um anexo image/tiff
    Quando o harness valida a entrada
    Então retorna erro de MIME não suportado
    E a sessão permanece ativa
  ```
- **Rastreabilidade:** FR-MM-001..006, FR-MM-002, FR-MM-011 · SC-MM-001/002.

---

## CU-MM-2 — Enviar documento (PDF / texto-CSV)

- **Identificador:** CU-MM-2
- **Escopo:** harness (núcleo + adaptador)
- **Nível:** objetivo do usuário
- **Ator primário:** Usuário
- **Atores de apoio:** Host, Harness, Provedor
- **Pré-condições:** documento em MIME da allowlist e dentro do teto.
- **Gatilho:** `Run` com `Part{kind:document, media:{mime, bytes|url, name}}`.
- **Garantia de sucesso:** PDF vai como documento nativo quando o perfil suporta; `text/plain`/`text/csv` vai inline como texto.
- **Garantia mínima:** binário não suportado é rejeitado com código estável; nada de mídia em log.
- **Fluxo principal de sucesso:**
  1. Host anexa o documento e chama `Run`.
  2. Harness valida allowlist/teto/fonte.
  3. Se `application/pdf` e `capabilities.documents=true`: mapeia para bloco de documento nativo.
  4. Se `text/plain`/`text/csv`: inline como texto (respeitando limite).
  5. Provedor responde; turno conclui.
- **Fluxos alternativos e de exceção:**
  - **3a. PDF sem `documents=true`:** erro nomeado (ou fallback textual se o host fornecer o texto).
  - **2a. xlsx/outro binário:** rejeitado (D-MM-4), orientando conversão no host.
  - **4a. CSV acima do limite textual:** truncado com registro (`truncated=true`).
- **Regras de negócio:** allowlist; capacidade; texto sempre permitido quando o MIME é textual.
- **Requisitos especiais (NFR):** redação (FR-MM-008); estimativa de contexto (FR-MM-009).
- **Dados e variações:** mime (pdf/text), nome, tamanho.
- **Questões em aberto:** L-MM-2 (bloco de arquivo no OpenAI-compatible).
- **Critérios de aceite (Gherkin):**
  ```gherkin
  Cenário: PDF nativo
    Dado um perfil com capabilities.documents=true
    E um PDF dentro do teto
    Quando o Host executa o turno
    Então o adaptador envia um bloco de documento nativo

  Cenário: CSV vira texto inline
    Dado um anexo text/csv
    Quando o harness monta o contexto
    Então o conteúdo textual é enviado como texto
    E o turno conclui

  Cenário: binário não suportado
    Dado um anexo xlsx
    Quando o harness valida
    Então retorna código estável orientando conversão no host
  ```
- **Rastreabilidade:** FR-MM-005/006/007 · CU-MM-2.

---

## CU-MM-3 — Redação de mídia na auditoria

- **Identificador:** CU-MM-3
- **Escopo:** harness (auditoria/redação)
- **Nível:** subfunção
- **Ator primário:** Operador
- **Atores de apoio:** Harness, AuditSink
- **Pré-condições:** turno com pelo menos uma Part de mídia.
- **Gatilho:** emissão de `AuditEvent`/log do turno.
- **Garantia de sucesso:** bytes/URL ausentes; mime/nome/tamanho presentes.
- **Fluxo principal de sucesso:** redator percorre o payload e substitui bytes/URL por `[REDACTED]`, mantendo metadados.
- **Fluxos alternativos e de exceção:** **1a. URL assinada** também é redigida; **1b. base64 em campo aninhado** é redigido por allowlist.
- **Regras de negócio:** redação por allowlist invariável (constitution §4).
- **Critérios de aceite (Gherkin):**
  ```gherkin
  Cenário: binário não vaza na trilha
    Dado um anexo de imagem com bytes inline
    Quando o evento de auditoria é emitido
    Então o payload aparece como [REDACTED]
    E mime e size_bytes permanecem visíveis
  ```
- **Rastreabilidade:** FR-MM-008 · SC-MM-003.

---

## CU-MM-4 — Fallback respeitando capacidade de mídia

- **Identificador:** CU-MM-4
- **Escopo:** harness (roteamento de modelo)
- **Ator primário:** Harness
- **Pré-condições:** turno com mídia; cadeia de fallback configurada.
- **Gatilho:** falha do perfil primário em turno com mídia.
- **Garantia de sucesso:** só um perfil com a capacidade exigida recebe a mídia; senão, erro explícito.
- **Critérios de aceite (Gherkin):**
  ```gherkin
  Cenário: fallback sem visão é ignorado
    Dado um turno com imagem e primário indisponível
    E o fallback não declara vision
    Quando o roteamento tenta o fallback
    Então o fallback é recusado para esse turno
    E o erro final preserva a causa
  ```
- **Rastreabilidade:** FR-MM-011.
