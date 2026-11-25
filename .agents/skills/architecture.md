---
name: "go-screaming-architecture"
description: "Estipula regras, convenções de nomenclatura e diretrizes arquiteturais para projetos Go usando Screaming Architecture, Clean Architecture e princípios Hexagonais. Use esta habilidade para estruturar pastas, gerar códigos de serviço, criar contratos de domínio e implementar interfaces idiomáticas em Go."
---

# 🛠️ Habilidade: Padronização e Arquitetura Go (Screaming Architecture)

Esta habilidade ensina o Claude a estruturar, organizar e padronizar o código de projetos Go utilizando os conceitos de **Screaming Architecture**, **Clean Architecture** e **Arquitetura Hexagonal**. O objetivo principal é garantir que a estrutura de diretórios e o código-fonte reflitam imediatamente o domínio do negócio em vez de tecnologias de infraestrutura ou frameworks.

---

## 📂 1. Estrutura Padrão de Pastas (Screaming Architecture)

Sempre que esta habilidade estiver ativa e for solicitado um novo projeto ou funcionalidade, a árvore de pastas deve seguir estritamente o layout abaixo (utilizando o domínio `book` como exemplo de referência):

```text
├── cmd/
│   ├── api/             # Entrypoint único da API REST (contém apenas o main.go)
│   └── appctl/          # Entrypoint para ferramentas de CLI, Workers ou scripts
├── config/              # Centralização de configurações e carregamento de variáveis de ambiente
└── internal/            # Encapsulamento obrigatório: impede importações por outros projetos
    ├── book/            # 🚨 PACOTE DE DOMÍNIO (Screaming Architecture - Foco de Negócio)
    │   ├── book.go      # Definição de structs de entidade e interfaces puras (UseCase, Repository)
    │   ├── service.go   # Implementação estrita das regras de negócio (UseCase)
    │   ├── service_test.go  # Testes unitários do domínio com mocks injetados
    │   ├── mock/        # Mocks das interfaces gerados para automação de testes
    │   └── postgres/    # Driven Adapter específico: persistência SQL (ex: PostgreSQL)
    │       └── book_storage.go
    ├── http/            # Driver Actors / Adapters (Protocolos de entrada)
    │   └── gin/         # Adapter de Protocolo: Rotas e Handlers HTTP (ex: usando Gin)
    │       └── handler.go
    └── event/
        └── kafka/       # Adapter de Mensageria: Consumidores ou Produtores (Kafka)
            └── event.go
```

> **Nota sobre `internal/`:** Em aplicações, serviços e CLIs que não expõem bibliotecas públicas, tanto o pacote de domínio (`internal/book/`) quanto os driver adapters (`internal/http/`, `internal/event/`) devem ficar dentro de `internal/` para proteger as regras de negócio de importações externas.

---

## 🎯 2. Instruções de Implementação para o Claude

### 2.1 Camada de Domínio (`internal/book/`)
* **`book.go` (Contratos Puros):**
  * Deve conter apenas structs que representam as entidades de negócio puras e as interfaces para casos de uso (`UseCase`) e persistência (`Repository`).
  * **Semântica de valor vs. ponteiro:** Entidades e objetos que representam dados (como `Book`) devem usar **semântica de valor** nos contratos e métodos; structs que representam comportamentos ou APIs (como `service` ou adapters) devem usar **ponteiros**.
  * **Regra Rígida:** Não são permitidos imports de frameworks, drivers de banco de dados ou pacotes de transporte (como HTTP) neste arquivo.
* **`service.go` (Lógica de Negócio):**
  * Deve implementar a interface `UseCase` definida em `book.go`.
  * Deve lidar apenas com lógica de negócio e validações puras.
  * Deve se comunicar com componentes externos unicamente por meio da interface `Repository` injetada. O serviço não sabe nem se importa se o banco real é Postgres, MongoDB ou em memória.
* **Isolamento entre Domínios:**
  * Pacotes de domínio vizinhos não devem depender diretamente uns dos outros para evitar acoplamento temporal e dependências cíclicas (*import cycle*). Toda orquestração ou chamadas sequenciais devem ser feitas na camada de entrada (`internal/http` ou similar).

### 2.2 Camada de Banco de Dados (`internal/book/postgres/`)
* Implementa a interface `Repository` especificada no domínio.
* Isola todo o acesso a queries SQL, ORMs, mapeamentos de tabelas e transações específicas da tecnologia de banco escolhida.

### 2.3 Camada de Protocolo e Transporte (`internal/http/`, `internal/event/`)
* Traduz requisições externas (JSON HTTP, payloads gRPC, eventos do Kafka) para chamadas aos métodos de `UseCase` do domínio.
* Nenhum objeto específico de framework (como `gin.Context` ou `echo.Context`) pode vazar ou ser importado na camada de domínio (`internal/book/`).

---

## 🚫 3. Guardrails e Restrições Rígidas (Anti-Patterns)

Fica estritamente proibido ao Claude gerar ou aceitar código com os seguintes padrões:

1. **Pacotes Genéricos ("Lixeira de Código"):**
   * É proibida a criação de pastas ou pacotes com os nomes `util`, `helper`, `common` ou `shared`. Se um código é compartilhado, dê um nome semântico com responsabilidade única (ex: `crypto`, `validator`, `jsonparser`).
2. **Stuttering (Gagueira na Nomenclatura):**
   * Não repita o nome do pacote na declaração de métodos, structs ou tipos.
   * ✅ *Correto:* `log.Info()`, `book.Service`
   * ❌ *Incorreto:* `log.LogInfo()`, `book.BookService`
3. **Getters e Setters Desnecessários:**
   * Evite utilizar a palavra `Get` no início de funções de acesso para atributos simples.
   * ✅ *Correto:* `customer.Name`
   * ❌ *Incorreto:* `customer.GetName()`
4. **Nomenclatura de Variáveis por Distância:**
   * Loops e variáveis de escopo micro: use apenas um caractere (`i`, `x`).
   * Slices e arrays de escopo curto: use letras repetidas no plural (`var bb []Book`).
   * Escopo macro/longo: use nomes descritivos e autoexplicativos.

---

## 📝 4. Exemplos de Referência (Few-Shot Prompting)

Ao gerar arquivos de domínio, utilize a estrutura sintática simplificada abaixo como modelo absoluto:

### Exemplo 4.1: `internal/book/book.go`
```go
package book

import "context"

// Book representa a entidade de domínio pura.
type Book struct {
	ID     string
	Title  string
	Author string
}

// UseCase define as operações de negócio permitidas.
type UseCase interface {
	Create(ctx context.Context, b Book) error
	FindByID(ctx context.Context, id string) (Book, error)
}

// Repository define o contrato abstrato para a persistência.
type Repository interface {
	Save(ctx context.Context, b Book) error
	GetByID(ctx context.Context, id string) (Book, error)
}
```

### Exemplo 4.2: `internal/book/service.go`
```go
package book

import (
	"context"
	"errors"
)

var ErrInvalidTitle = errors.New("o título do livro não pode ser vazio")

type service struct {
	repo Repository
}

// NewService injeta o repositório e retorna a interface do caso de uso.
func NewService(r Repository) UseCase {
	return &service{
		repo: r,
	}
}

func (s *service) Create(ctx context.Context, b Book) error {
	if b.Title == "" {
		return ErrInvalidTitle
	}
	return s.repo.Save(ctx, b)
}

func (s *service) FindByID(ctx context.Context, id string) (Book, error) {
	return s.repo.GetByID(ctx, id)
}
```