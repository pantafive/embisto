# Operations

## Metrics

`EMBISTO_METRICS_ADDR` (`:9090`) serves Prometheus metrics at `/metrics`, apart from the API. The metrics cover:

- HTTP requests by route and status;
- outgoing fetches by stage and outcome;
- provider matches;
- cache lookups (hit, miss, or stale: found past its expiry), requests coalesced in the instance or across instances, shared locks (acquired, waited, timeout, error), where each TTL came from (origin, min, max, default, failure), and whether the shared cache answers;
- thumbnails served, by outcome;
- every card made afresh, by class and by the provider that shaped it (or `generic`).

No label carries a URL. A card's class is one of:

| Class | Meaning |
|---|---|
| `full` | title, description and a picture or player; or a picture or video the URL serves itself |
| `partial` | title and one of a description and a picture |
| `minimal` | less, often only a title |
| `wall` | the site answered with a wall, and the card is `limited` |
| `error` | no card |

Each card below partial is logged as `poor card` with its host, class, provider or `generic`, and failure code. Ranking hosts by those lines shows which sites a provider would help most.

## Debugging a card

With `EMBISTO_DEBUG=true`, the internal listener (`:9090`, beside `/metrics`) serves `/debug?url=`. Paste a URL to see:

- the card it makes afresh, without the cache and outside the metrics;
- every fetch, source, provider and enricher, with what each found, its outcome and its time;
- each field's candidates in the order they were asked, the source that won, and why; boilerplate dropped from a field is listed too;
- when thumbnails are on, the card's image in the `card` preset in both formats, with bytes and time.

Everything a page says is shown escaped, never run. The page is for operators: the API's own listener never serves it.

## Stopping

On SIGTERM or an interrupt, the server answers `/readyz` with 503 for 5 seconds while it still serves, so the load balancer stops sending. It then stops taking requests and gives those in flight up to 24 seconds, longer than the slowest answer. The whole stop fits Kubernetes' default termination grace period of 30 seconds; a shorter one cuts answers off.
