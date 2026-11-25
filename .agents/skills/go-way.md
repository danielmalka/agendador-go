---
name: "go-way"
description: "Estipula regras para desenvolvimento de aplicações em Go."
---

Go Way

1. Estrutura e Design de Pacotes

- Pacotes como provedores: Pacotes devem fornecer algo (uma funcionalidade de negócio, como `internal/book`) em vez de apenas conter arquivos genéricos (como `models` ou `utils`).
- Arquitetura que grita: A estrutura do diretório deve indicar claramente o domínio do projeto (ex: `internal/book`) e não a tecnologia utilizada.
- Proteção de código: Utilize o pacote `internal` para encapsular regras de negócio (domínio e seus driven adapters, como `internal/book/` e `internal/book/postgres/`) e driver adapters (`internal/http/`, `internal/event/`) que não devem ser importados por outros projetos.

2. Tipagem e Lógica de Negócio

- Tipos específicos: Use o sistema de tipos a seu favor, criando tipos customizados para domínios importantes (ex: type category int) para aumentar a legibilidade e segurança.
- Semântica de valor vs. ponteiros:
 - Use semântica de valor para objetos que representam dados e entidades (como `Book`), inclusive em assinaturas de `UseCase` e `Repository`.
 - Use ponteiros para objetos que representam comportamentos, serviços ou APIs (como `*service`, `Service` ou adapters de `Repository`).

3. Interfaces e Composição

- Interfaces pequenas: Prefira interfaces pequenas (ex: Reader, Writer) que podem ser compostas para formar comportamentos mais complexos.
- Programação para interfaces: O código deve depender de interfaces, não de implementações concretas, facilitando a substituição e o teste.

4. Tratamento de Erros e Contexto

- Contexto obrigatório: Todas as funções de negócio devem receber context.Context como primeiro parâmetro para permitir traces e timeouts.
- Erros como retorno: O último valor de retorno deve sempre ser um error.
- Fluxo de saída: Os erros não devem ser tratados em camadas intermediárias; eles devem ser retornados até a camada principal (main), que serve como porta de entrada e saída da aplicação.

5. Camadas e Transformação

- Separação de responsabilidades: Camadas de domínio devem ser puras (sem tags JSON), enquanto camadas externas (HTTP) cuidam das transformações de dados (37:30 - 38:07).
- Unidirecionalidade: Importações devem seguir uma única direção (ex: main > http > service > repository), evitando dependências circulares.
