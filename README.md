# agendador-go

Agendador de tarefas em Go. Lê um arquivo YAML com a lista de jobs e executa cada um conforme sua expressão cron: scripts Python, CLIs ou qualquer executável com argumentos de linha de comando.

- Um único binário, sem banco de dados e sem servidor HTTP.
- Os comandos são executados diretamente via `os/exec`, **nunca por um shell**.
- Cada job roda em seu próprio *process group* (Unix), então um timeout ou um shutdown mata também os processos filhos e netos.
- Se uma execução ainda está em andamento quando chega o próximo trigger do mesmo job, o novo trigger é pulado.

## Sumário

1. [Requisitos](#requisitos)
2. [Instalação e build](#instalação-e-build)
3. [Uso](#uso)
4. [How To](#how-to)
5. [Referência do YAML](#referência-do-yaml)
6. [Expressões de schedule](#expressões-de-schedule)
7. [Comportamento em execução](#comportamento-em-execução)
8. [Shutdown](#shutdown)
9. [Logs](#logs)
10. [Rodando como serviço](#rodando-como-serviço)
11. [Segurança](#segurança)
12. [Arquitetura](#arquitetura)
13. [Desenvolvimento](#desenvolvimento)
14. [Limitações e fora de escopo](#limitações-e-fora-de-escopo)

## Requisitos

- Go `1.27.1` ou superior (campo `go` do `go.mod`), apenas para compilar.
- Linux ou macOS. O isolamento por process group (`Setpgid` e kill do grupo) existe só em sistemas Unix.
- Python 3 no `PATH` (ou outro interpretador informado em `interpreter`), apenas se você usar jobs `type: python`.
- Windows: o código tem um caminho alternativo (`internal/scheduling/exec/runner_other.go`) que mata somente o processo direto, sem os netos. Esse caminho não é exercitado pelos testes do projeto; trate o Windows como não suportado.

## Instalação e build

```sh
make build        # gera bin/scheduler
```

O binário fica em `bin/scheduler`. Equivalente sem o Makefile:

```sh
go build -o bin/scheduler ./cmd/scheduler
```

Para rodar sem gerar o binário (o `make run` usa `configs/jobs.yml`):

```sh
cp configs/jobs.example.yml configs/jobs.yml
make run
# ou
go run ./cmd/scheduler -config configs/jobs.yml
```

## Uso

```sh
./bin/scheduler -config configs/jobs.yml -shutdown-timeout 30s -log-level info
```

| Flag | Padrão | Validação |
|---|---|---|
| `-config` | `configs/jobs.yml` | Caminho do YAML de jobs. Arquivo inexistente ou inválido encerra com erro. |
| `-shutdown-timeout` | `30s` | Duração Go (`30s`, `2m`...). Precisa ser maior que zero; caso contrário: `usage: -shutdown-timeout must be greater than zero`. |
| `-log-level` | `info` | `debug`, `info`, `warn` ou `error` (sem diferenciar maiúsculas). Outro valor: `usage: -log-level must be debug, info, warn or error`. |

Não há outras flags. O processo roda em primeiro plano até receber `SIGINT` ou `SIGTERM`.

### Exit codes do agendador

| Código | Quando |
|---|---|
| `0` | Shutdown por sinal em que todos os jobs em execução terminaram dentro do `-shutdown-timeout`; ou `-h`/`-help`, que imprime só o usage, sem a linha `error:`. |
| `1` | Qualquer erro: flag inválida, YAML inválido, schedule inválido, `-shutdown-timeout` esgotado com jobs ainda rodando, ou segundo sinal. A mensagem vai para o stderr com o prefixo `error:`. |

## How To

Exemplos práticos de tudo o que pode ser configurado e executado com o `agendador-go`.

### 1. Rodar um script Python com argumentos

Use `type: python` informando o caminho do script em `command` e os argumentos em `args`. Por padrão, o agendador executa com `python3`:

```yaml
jobs:
  # Executa: python3 scripts/hello.py --name world
  - name: hello-python
    schedule: "*/1 * * * *"
    type: python
    command: scripts/hello.py
    args: ["--name", "world"]
```

### 2. Usar um virtualenv ou versão específica do Python

Informe o caminho completo ou o binário desejado em `interpreter` (opcionalmente combinando com `workdir`):

```yaml
jobs:
  # Executa dentro de /srv/etl: /srv/etl/.venv/bin/python pipeline.py --full
  - name: etl-venv
    schedule: "0 3 * * *"
    type: python
    interpreter: /srv/etl/.venv/bin/python
    command: pipeline.py
    args: ["--full"]
    workdir: /srv/etl
```

### 3. Executar qualquer CLI, script binário ou ferramenta do sistema

Use `type: command` para chamar qualquer executável no `PATH` ou por caminho absoluto, passando flags em `args` (sem interpretação de shell):

```yaml
jobs:
  # Executa: curl -fsS https://api.exemplo.local/health
  - name: healthcheck
    schedule: "@every 30s"
    type: command
    command: curl
    args: ["-fsS", "https://api.exemplo.local/health"]
    timeout: 10s
```

### 4. Escolher diferentes formatos de agendamento

Você pode combinar expressões cron de 5 campos, 6 campos (segundos no início), descritores e timezone por job:

```yaml
jobs:
  # A cada 15 minutos
  - name: sync-15m
    schedule: "*/15 * * * *"
    type: command
    command: /usr/local/bin/sync-dados

  # Todo dia útil às 08:30 (horário de São Paulo, independente do TZ do host)
  - name: abertura-sp
    schedule: "CRON_TZ=America/Sao_Paulo 30 8 * * 1-5"
    type: python
    command: scripts/abertura.py

  # Com segundos no início: a cada 5 minutos, exatamente no segundo 10
  - name: coleta-precisa
    schedule: "10 */5 * * * *"
    type: command
    command: /usr/local/bin/coletar

  # Descritor de intervalo fixo
  - name: heartbeat
    schedule: "@every 1m30s"
    type: command
    command: echo
    args: ["alive"]
```

### 5. Definir timeout para evitar jobs travados

Configure `timeout` com qualquer duração Go válida (`10s`, `5m`, `1h30m`). Se o tempo estourar, todo o *process group* (incluindo processos filhos e netos) recebe `SIGKILL`:

```yaml
jobs:
  - name: importacao-limitada
    schedule: "@hourly"
    type: python
    command: scripts/importar.py
    timeout: 5m
```

### 6. Injetar variáveis de ambiente e definir diretório de trabalho

Use `env` para adicionar ou sobrescrever variáveis herdadas pelo processo e `workdir` para rodar o comando a partir de um diretório específico:

```yaml
jobs:
  - name: relatorio-csv
    schedule: "@daily"
    type: command
    command: ./gerar-relatorio
    args: ["--saida", "out/diario.csv"]
    workdir: /srv/relatorios
    env:
      LC_ALL: C
      APP_ENV: production
```

### 7. Usar recursos de shell como pipes e redirecionamentos

Por segurança, o agendador nunca passa comandos por um shell. Quando você realmente precisar de pipes (`|`), redirecionamentos (`>`), `&&` ou expansão de variáveis (`$VAR`), invoque `sh` explicitamente com `-c`:

```yaml
jobs:
  - name: contar-erros
    schedule: "@hourly"
    type: command
    command: sh
    args: ["-c", "grep -c ERROR /var/log/app.log > /tmp/erros.txt"]
```

> **Atenção:** com `sh -c`, nunca interpole dados não confiáveis na string do comando. Sempre que possível, prefira encapsular a lógica em um script dedicado (`type: python`).

### 8. Depurar saída (`stdout` e `stderr`) dos jobs pelos logs

Em nível padrão (`info`), execuções bem-sucedidas não exibem `stdout`/`stderr` e falhas exibem apenas `stderr`. Para inspecionar `stdout` e `stderr` completos (até 4 KiB nos logs), rode com `-log-level debug`:

```sh
./bin/scheduler -config configs/jobs.yml -log-level debug
```

### 9. Ajustar o tempo de espera no encerramento (graceful shutdown)

Se você tiver jobs longos que precisam de mais que os `30s` padrão para finalizar ao receber `SIGTERM`/`SIGINT`, aumente `-shutdown-timeout`:

```sh
./bin/scheduler -config configs/jobs.yml -shutdown-timeout 2m
```

- Enviar um **1º sinal** (`Ctrl+C` ou `SIGTERM`) interrompe novos disparos e aguarda os jobs em andamento até `2m`.
- Enviar um **2º sinal** aborta a espera imediatamente, mata os jobs em execução e encerra com código `1`.

### 10. Validação automática na inicialização

O agendador não possui uma flag dedicada de *dry-run*, mas valida todo o arquivo antes de iniciar: sintaxe YAML, campos desconhecidos, nomes duplicados, `timeout` e todas as expressões de `schedule`. Se houver qualquer erro, ele imprime `error: load config: ...` no stderr e encerra imediatamente com código `1` antes de agendar qualquer job.

## Referência do YAML

O arquivo tem uma chave raiz `jobs`, uma lista. Campos desconhecidos são **rejeitados** (o parse é estrito), assim como um arquivo sem nenhum job.

| Campo | Tipo | Obrigatório | Padrão | Validação |
|---|---|---|---|---|
| `name` | string | sim | - | Não vazio e único no arquivo. |
| `schedule` | string | sim | - | Expressão aceita pelo parser (veja [Expressões de schedule](#expressões-de-schedule)). |
| `type` | string | sim | - | `python` ou `command`. |
| `command` | string | sim | - | Não vazio. Em `python`, é o caminho do script; em `command`, o executável. |
| `args` | lista de strings | não | vazia | Cada item vira um argumento, sem interpretação de shell. |
| `interpreter` | string | não | `python3` | Só tem efeito em `type: python`; é ignorado em `command`. |
| `timeout` | string | não | vazio (sem limite) | Duração Go (`30s`, `5m`, `1h30m`). Negativa é erro; `0` ou vazio significa sem limite. |
| `env` | mapa string para string | não | vazio | Variáveis somadas ao ambiente do agendador; em caso de mesma chave, a do job prevalece. |
| `workdir` | string | não | vazio | Diretório de trabalho do processo. Vazio herda o diretório do agendador. |

Como o comando é montado:

- `type: python` executa `<interpreter> <command> <args...>`.
- `type: command` executa `<command> <args...>`.

### Exemplo comentado

```yaml
jobs:
  # Script Python com argumentos e timeout.
  # Executa: python3 scripts/hello.py --name world
  - name: hello-python
    schedule: "*/1 * * * *"       # a cada minuto
    type: python
    command: scripts/hello.py
    args: ["--name", "world"]
    timeout: 30s

  # Python com interpretador específico e diretório de trabalho.
  # Executa (dentro de /srv/relatorios): python3.12 gerar.py --formato csv
  - name: relatorio-diario
    schedule: "0 6 * * *"         # todo dia às 06:00
    type: python
    interpreter: python3.12
    command: gerar.py             # resolvido a partir de workdir
    args: ["--formato", "csv"]
    workdir: /srv/relatorios
    timeout: 10m

  # CLI simples, sem shell. Executa: echo tick
  - name: echo-cli
    schedule: "@every 30s"
    type: command
    command: echo
    args: ["tick"]

  # Aplicação com flags, variáveis de ambiente, workdir e timeout.
  # Segundos opcionais: "0 */5 * * * *" = a cada 5 minutos, no segundo 0.
  - name: list-with-flags
    schedule: "0 */5 * * * *"
    type: command
    command: ls
    args: ["-l", "-a"]
    workdir: /tmp
    timeout: 1m
    env:
      LC_ALL: C
```

### Erros de validação

Os erros aparecem no stderr como `error: load config: ...` e o agendador **não inicia**. Mensagens por job vêm no formato `job "<nome>": <motivo>` (ou `job "#<posição>"` quando o `name` está vazio).

| Situação | Mensagem (trecho) |
|---|---|
| Arquivo não abre | `open config: ...` |
| YAML malformado ou campo desconhecido | `parse <arquivo>: ...` |
| Arquivo vazio ou sem jobs | `<arquivo>: no jobs defined` |
| `name` vazio | `name is required` |
| `schedule` vazio | `schedule is required` |
| `command` vazio | `command is required` |
| `type` ausente ou inválido | `unknown type "..." (want "python" or "command")` |
| `timeout` negativo | `timeout must not be negative` |
| `timeout` ilegível (ex.: `5 min`, `1d`) | `invalid timeout: ...` |
| `schedule` inválido | `invalid schedule "...": ...` |
| Nome repetido | `job "<nome>": duplicate name` |

## Expressões de schedule

O parser (`robfig/cron/v3`) aceita:

- **5 campos**: `minuto hora dia-do-mês mês dia-da-semana`, por exemplo `*/15 * * * *`.
- **6 campos** (segundos opcionais, **no início**): `segundo minuto hora dia-do-mês mês dia-da-semana`, por exemplo `0 */5 * * * *`.
- **Descritores**: `@yearly` (`@annually`), `@monthly`, `@weekly`, `@daily` (`@midnight`), `@hourly` e `@every <duração>` (por exemplo `@every 30s`, `@every 1h30m`).

Escreva sempre a expressão entre aspas no YAML (`"*/5 * * * *"`), pois `*` e `@` têm significado especial em YAML.

**Timezone:** o agendador usa o timezone local do processo (`time.Local`, ou seja, o `TZ` do ambiente ou `/etc/localtime`). O parser da biblioteca também aceita um prefixo por expressão, como `CRON_TZ=America/Sao_Paulo 0 9 * * *` ou `TZ=UTC 0 9 * * *`; esse prefixo vem da biblioteca e não é exercitado pelos testes do projeto. Em container, o timezone padrão costuma ser UTC.

## Comportamento em execução

- **Overlap é pulado.** Se o job ainda está rodando no próximo trigger, o trigger é descartado e é logado em `WARN` (`job skipped: previous run still in progress`). Não há queue.
- **Timeout.** Ao estourar `timeout`, o process group inteiro recebe `SIGKILL`. O job é logado como falha com `error="job interrupted: context deadline exceeded"` e `exit_code=-1`.
- **Exit code.** Código diferente de zero é uma falha (`job failed`, log level `ERROR`). `exit_code=-1` indica que o processo foi morto por sinal ou nunca iniciou (por exemplo, executável inexistente ou `workdir` inválido; o motivo vai no campo `error`).
- **Captura de output.** Stdout e stderr são capturados separadamente, **64 KiB cada**. Acima disso mantêm-se os primeiros 64 KiB e o resto é descartado em silêncio (o processo filho não trava por pipe cheio). Depois, o log aplica um corte adicional de 4 KiB (veja [Logs](#logs)).
- **Ambiente.** O processo herda o ambiente do agendador; `env` do job soma ou sobrescreve variáveis.
- **Sem shell.** Pipes (`|`), redirecionamentos (`>`), `&&`, `$VAR`, `~` e curingas (`*`) **não** são interpretados: viram argumentos literais. Caso precise explicitamente de shell, veja o exemplo com `sh -c` na seção [How To](#7-usar-recursos-de-shell-como-pipes-e-redirecionamentos).
- **Panic.** Um panic dentro da execução de um job é recuperado e logado em `ERROR` (`job panicked`); o agendador e os demais jobs continuam.

## Shutdown

1. **Primeiro sinal** (`SIGINT` ou `SIGTERM`): o agendador para de disparar novos jobs e espera os que estão rodando terminarem, por até `-shutdown-timeout`.
2. **Se o prazo esgotar**: os jobs ainda ativos são mortos (`SIGKILL` no process group inteiro, filhos e netos incluídos) e o agendador sai com código `1`.
3. **Segundo sinal**: aborta a espera na hora, mata os jobs em execução e sai com código `1`. Os jobs rodam em process groups próprios e não recebem o Ctrl+C do terminal; por isso o agendador os mata explicitamente em vez de deixar o processo morrer e orfanar os filhos.

Se todos os jobs terminarem dentro do prazo, o exit code é `0`.

## Logs

O agendador usa `log/slog` em formato texto (`chave=valor`) e escreve os logs no **stdout**. Erros fatais de inicialização vão para o stderr.

| Log level | Mensagens |
|---|---|
| `debug` | `job output`: stdout (e, em execuções bem-sucedidas, também stderr) de cada execução. |
| `info` | `job registered`, `scheduler started`, `job started`, `job finished` (com `exit_code` e `duration`), `shutting down`. |
| `warn` | `job skipped: previous run still in progress`. |
| `error` | `job failed` (com `exit_code`, `duration`, `error` e `stderr`) e `job panicked`. |

Nas falhas, o stderr aparece no log `ERROR` e o stdout só no `DEBUG`. Em execuções bem-sucedidas, nenhum output aparece abaixo do log level `debug`.

Cada campo de output (`stdout`, `stderr`) é cortado em **4 KiB** nos logs, respeitando o limite de caractere UTF-8, e termina com `...(truncated)`.

## Rodando como serviço

### systemd

`/etc/systemd/system/agendador.service`:

```ini
[Unit]
Description=Agendador de jobs
After=network.target

[Service]
User=agendador
ExecStart=/opt/agendador/bin/scheduler -config /etc/agendador/jobs.yml -shutdown-timeout 30s
WorkingDirectory=/opt/agendador
KillSignal=SIGTERM
# O padrão (control-group) manda SIGTERM a todo o cgroup, inclusive aos jobs (que rodam em process
# groups próprios), e atropela o graceful shutdown. Com mixed, o SIGTERM vai só ao
# agendador e o SIGKILL final do systemd vai a todo o cgroup.
KillMode=mixed
# Maior que -shutdown-timeout, para o agendador terminar sozinho antes do SIGKILL do systemd.
TimeoutStopSec=45s
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now agendador
journalctl -u agendador -f
```

### Docker

Exemplo mínimo (apenas um trecho ilustrativo; ajuste a imagem do Go para a versão do seu `go.mod`). A imagem final inclui o Python para jobs `type: python`:

```dockerfile
FROM golang:1.27 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /scheduler ./cmd/scheduler

FROM python:3-slim
COPY --from=build /scheduler /usr/local/bin/scheduler
COPY scripts/ /app/scripts/
WORKDIR /app
ENTRYPOINT ["scheduler"]
CMD ["-config", "/etc/agendador/jobs.yml"]
```

```sh
docker build -t agendador .
docker run --rm \
  -v "$PWD/configs/jobs.yml:/etc/agendador/jobs.yml:ro" \
  -e TZ=America/Sao_Paulo \
  --init \
  agendador
```

Use `--init` (ou `tini`): sem um init, o agendador vira o PID 1 e não recolhe netos órfãos, que viram *zombies*. No Compose, o equivalente é `init: true` no serviço.

O `docker stop` envia `SIGTERM` ao PID 1 e espera 10 s por padrão; para usar um `-shutdown-timeout` maior, passe `--stop-timeout` (ou `stop_grace_period` no Compose). Se a imagem não tiver dados de timezone (`tzdata`), o `TZ` pode não ter efeito.

## Segurança

- **O YAML é confiável.** Quem pode editá-lo executa comandos arbitrários com as permissões do usuário que roda o agendador. Restrinja a escrita no arquivo (por exemplo, `chmod 640`, dono `root` ou o usuário do serviço) e no diretório dos scripts.
- Rode o agendador com um usuário sem privilégios, nunca como root sem necessidade.
- O ambiente do agendador é herdado pelos jobs; não deixe segredos que eles não precisam nesse ambiente.
- Os logs podem conter trechos de stdout/stderr dos jobs (até 4 KiB por campo). Se um job imprime dados sensíveis, esses dados chegam ao log (stderr em falhas e, em `debug`, também o stdout).

## Arquitetura

```
cmd/scheduler                  composition root: sinais, logger, injeção das dependências e saída de eventos/erros
config                         carregamento e validação das flags/configurações da CLI
internal/scheduling            domínio: entidades (Job, Result, Event), erros sentinela, ports e service (UseCase)
internal/scheduling/yaml       driven adapter YAML da port JobLoader
internal/scheduling/exec       driven adapter os/exec da port Runner
internal/scheduling/cron       driven adapter robfig/cron das ports Scheduler e ScheduleValidator
internal/scheduling/mock       test doubles das ports do domínio
```

Ports (`internal/scheduling/scheduling.go`):

- `UseCase`: `Start(ctx, configPath)`, `Stop(ctx)`
- `JobLoader`: `Load(ctx, path) ([]Job, error)`
- `ScheduleValidator`: `Validate(ctx, s) error`
- `Runner`: `Run(ctx, j) (Result, error)`
- `Scheduler`: `Add(ctx, j, fn)`, `Start(ctx)`, `Stop(ctx)`
- `Observer`: `Observe(ctx, e)`

A direção das dependências é sempre dos adapters **para** o pacote de domínio `internal/scheduling`; o domínio não importa nenhum outro pacote do projeto nem frameworks/loggers. Só `cmd/scheduler` conhece os adapters concretos e traduz `Event`/erros para logs (`slog`) e código de saída. O loader recebe a validação de schedule por injeção (`ScheduleValidator`, implementada por `cron.Validator`), mantendo-o independente da biblioteca de cron.

### Como acrescentar um novo tipo de job

1. Em `internal/scheduling/scheduling.go`, adicione a constante `JobType` e aceite-a em `Job.Validate()`.
2. Em `Job.Argv()` (mesmo arquivo), devolva o executável e os argumentos desse tipo.
3. Se o tipo precisar de campo novo no YAML, acrescente-o em `jobDTO` e em `toJob` (`internal/scheduling/yaml/loader.go`) e em `scheduling.Job`.
4. Acrescente testes em `internal/scheduling/job_test.go` e `internal/scheduling/yaml/loader_test.go`, documente o tipo neste README e atualize `configs/jobs.example.yml`.

O runner (`internal/scheduling/exec`) não precisa mudar enquanto o novo tipo se resumir a "um executável com argumentos".

## Desenvolvimento

| Alvo | O que faz |
|---|---|
| `make build` | Compila para `bin/scheduler`. |
| `make test` | `go test ./... -count=1`, com `-race` quando há cgo e `gcc`. |
| `make cover` | Gera `coverage.out` e **falha se o coverage total for menor que 80%**. |
| `make vet` | `go vet ./...`. |
| `make run` | `go run ./cmd/scheduler -config configs/jobs.yml`. |

Os testes do executor (`internal/scheduling/exec`) dependem de utilitários Unix (por exemplo, `sh`, `sleep`, `echo`) e dos recursos de process group; não rodam em Windows.

## Limitações e fora de escopo

- Sem hot reload: alterar o YAML exige reiniciar o agendador.
- Sem retry automático de jobs que falharam.
- Sem API HTTP e sem metrics.
- Sem persistência: não há histórico de execuções nem recuperação de triggers perdidos enquanto o agendador estava parado.
- Sem queue: triggers que coincidem com uma execução em andamento são descartados.
- Sem execução por shell; veja [Como usar `sh -c` no How To](#7-usar-recursos-de-shell-como-pipes-e-redirecionamentos) para o caso em que você a quer explicitamente.
