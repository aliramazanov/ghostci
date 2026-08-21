# Corpus sources

Each directory holds an unmodified copy of the `.github` directory of the
public repository it is named after: `BurntSushi_ripgrep` is
github.com/BurntSushi/ripgrep. They are test input for the importer, used to
measure how much of a real workflow it can extract, and nothing here is built
or executed.

Every file remains under its original project's license, listed below. Only
permissively licensed projects are included, so that vendoring them alongside
this repository's MIT license carries no further obligation than attribution.
Copyleft and source-available projects were deliberately left out, which is why
Grafana (AGPL-3.0), Elasticsearch (AGPL-3.0 / SSPL-1 / Elastic-2.0) and
Terraform (BUSL-1.1) are absent despite being useful examples.

| Directory | Upstream | License |
| --- | --- | --- |
| `apache_kafka` | apache/kafka | Apache-2.0 |
| `astral-sh_ruff` | astral-sh/ruff | MIT |
| `astral-sh_uv` | astral-sh/uv | Apache-2.0 |
| `BurntSushi_ripgrep` | BurntSushi/ripgrep | Unlicense |
| `cli_cli` | cli/cli | MIT |
| `denoland_deno` | denoland/deno | MIT |
| `django_django` | django/django | BSD-3-Clause |
| `evilmartians_lefthook` | evilmartians/lefthook | MIT |
| `facebook_react` | facebook/react | MIT |
| `fastapi_fastapi` | fastapi/fastapi | MIT |
| `j178_prek` | j178/prek | MIT |
| `microsoft_TypeScript` | microsoft/TypeScript | Apache-2.0 |
| `moonrepo_moon` | moonrepo/moon | MIT |
| `pallets_flask` | pallets/flask | BSD-3-Clause |
| `pnpm_pnpm` | pnpm/pnpm | MIT |
| `prometheus_prometheus` | prometheus/prometheus | Apache-2.0 |
| `psf_requests` | psf/requests | Apache-2.0 |
| `pydantic_pydantic` | pydantic/pydantic | MIT |
| `rust-lang_rust` | rust-lang/rust | Apache-2.0 |
| `sharkdp_bat` | sharkdp/bat | Apache-2.0 |
| `spring-projects_spring-boot` | spring-projects/spring-boot | Apache-2.0 |
| `sveltejs_svelte` | sveltejs/svelte | MIT |
| `tokio-rs_tokio` | tokio-rs/tokio | MIT |
| `vercel_next.js` | vercel/next.js | MIT |
| `vitejs_vite` | vitejs/vite | MIT |

Apache-2.0 requires that its notice travel with the copied files; this table
is that notice. To add a project, check its license first and record it here.
