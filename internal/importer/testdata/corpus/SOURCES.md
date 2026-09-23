# Corpus sources

Each directory holds an unmodified copy of the `.github` directory of the
public repository it is named after: `BurntSushi_ripgrep` is
github.com/BurntSushi/ripgrep. They are test input for the importer, used to
measure how much of a real workflow it can extract, and nothing here is built
or executed.

Every file remains under its original project's license, listed below. Each
directory also carries that project's own license file, unchanged, and its
NOTICE file where the project has one, because MIT, BSD and Apache-2.0 all ask
that the license and copyright notices travel with any copy. Only permissively
licensed projects are included, so vendoring them alongside this repository's
MIT license asks nothing more than that.
Copyleft and source-available projects were deliberately left out, which is why
Grafana (AGPL-3.0), Elasticsearch (AGPL-3.0 / SSPL-1 / Elastic-2.0) and
Terraform (BUSL-1.1) are absent despite being useful examples.

| Directory | Upstream | License |
| --- | --- | --- |
| `apache_kafka` | apache/kafka | Apache-2.0 |
| `astral-sh_ruff` | astral-sh/ruff | MIT |
| `astral-sh_uv` | astral-sh/uv | Apache-2.0 |
| `BurntSushi_ripgrep` | BurntSushi/ripgrep | Unlicense |
| `clap-rs_clap` | clap-rs/clap | Apache-2.0 |
| `cli_cli` | cli/cli | MIT |
| `denoland_deno` | denoland/deno | MIT |
| `django_django` | django/django | BSD-3-Clause |
| `evilmartians_lefthook` | evilmartians/lefthook | MIT |
| `expressjs_express` | expressjs/express | MIT |
| `facebook_react` | facebook/react | MIT |
| `fastapi_fastapi` | fastapi/fastapi | MIT |
| `gin-gonic_gin` | gin-gonic/gin | MIT |
| `google_gson` | google/gson | Apache-2.0 |
| `j178_prek` | j178/prek | MIT |
| `microsoft_TypeScript` | microsoft/TypeScript | Apache-2.0 |
| `moonrepo_moon` | moonrepo/moon | MIT |
| `pallets_click` | pallets/click | BSD-3-Clause |
| `pallets_flask` | pallets/flask | BSD-3-Clause |
| `pnpm_pnpm` | pnpm/pnpm | MIT |
| `prometheus_prometheus` | prometheus/prometheus | Apache-2.0 |
| `psf_requests` | psf/requests | Apache-2.0 |
| `pydantic_pydantic` | pydantic/pydantic | MIT |
| `rubocop_rubocop` | rubocop/rubocop | MIT |
| `rust-lang_rust` | rust-lang/rust | Apache-2.0 |
| `sharkdp_bat` | sharkdp/bat | Apache-2.0 |
| `spring-projects_spring-boot` | spring-projects/spring-boot | Apache-2.0 |
| `sveltejs_svelte` | sveltejs/svelte | MIT |
| `tokio-rs_tokio` | tokio-rs/tokio | MIT |
| `vercel_next.js` | vercel/next.js | MIT |
| `vitejs_vite` | vitejs/vite | MIT |

Where a project offers a choice of licenses, the one listed is the one whose
text is copied. To add a project, check its license first, copy its license
file and any NOTICE file into its directory, and record it here.
