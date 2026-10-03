# FIAS / GAR API — Russian address autocomplete and normalization

[Русский](README.md) · **English**

[![Live API tests](https://github.com/atlorium-api/gar-fias-address-api-client/actions/workflows/examples.yml/badge.svg)](https://github.com/atlorium-api/gar-fias-address-api-client/actions/workflows/examples.yml)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![API](https://img.shields.io/badge/API-Swagger-brightgreen)](https://atlorium.com/garAPI)

Ready-to-run examples for the **FIAS address API** (GAR — the Russian State Address Register) in six languages: **Python, TypeScript (Node.js), Go, Java, C#, PHP.**
**Russian address autocomplete** as the user types, full search over the **Russian address database**, **address normalization** down to a FIAS code, plus **postal code lookup**, OKTMO, OKATO, KLADR code and tax office (IFNS) code.

The service runs against a **local copy of the GAR database** — no calls to third-party sources, which is why responses come back in single-digit milliseconds.

Every example **runs out of the box — no signup, no key, no card.** A public demo key is baked in.

```bash
git clone https://github.com/atlorium-api/gar-fias-address-api-client
cd gar-fias-address-api-client/python && pip install -r requirements.txt && python main.py
```

> The demo key returns **realistic mock data**, not the real GAR register — which is why a query for "москва тверская" (Moscow, Tverskaya) comes back with addresses in Novosibirsk. The [sandbox section below](#what-exactly-is-wrong-with-the-sandbox) is honest about every quirk. Swap in a live key and the same code hits the real FIAS/GAR database.

---

## What it is for

Checkout, delivery and signup forms where a user types an address. Cleaning addresses in a CRM. Filling government and tax forms that require OKTMO and an IFNS code. Logistics that needs a postal code. Anywhere an address is not just a string but an entity that must be reconciled with a state register.

The examples do not just print JSON — they **apply** it. Each ships a `suggestAddress()` function that takes autocomplete all the way through, in the same two steps a real frontend + backend pair performs:

1. **What the user sees.** Take the prefix they typed, call `/api/Gar/suggest`, print a numbered list — that is the dropdown under the input field.
2. **What the backend stores.** Take the `objectGuid` of the **selected** suggestion, call `/api/Gar/object/{guid}`, and get the canonical address, its hierarchy and its codes.

### The point: never store the string the user typed

Store the **`objectGuid`** — the FIAS code, a stable identifier for the address object. Alongside it, denormalized: `fullAddress`, postal code, OKTMO, OKATO, IFNS code, KLADR code. What that buys you:

- **Addresses stop drifting apart.** "ул. Лесная", "улица Лесная" and "Лесная ул." are one `objectGuid`, not three separate rows. Deduplicating customers, warehouses and delivery points stops being archaeology.
- **You never ask the user for a postal code or OKTMO** — they arrive with the address. Fewer form fields, higher conversion.
- **IFNS and OKTMO codes are required for tax reporting and government forms.** Nobody fills those in correctly by hand; GAR supplies them for free.

## What exactly is wrong with the sandbox

The demo key is a **generator of plausible data, not a copy of the real register.** We did not tune the example to produce a pretty output, so you will see exactly what we see:

| What happens | Why |
|---|---|
| Suggestions are **not relevant to the query** — "москва тверская" returns addresses in Novosibirsk | The mock does not search; it generates plausible addresses from a seed derived from the query |
| Region and city **contradict each other** within one line ("Republic of Tatarstan, city of Penza") | Address components are generated independently |
| The `limit` parameter is **ignored** — ask for 3, get 7 | The mock returns a fixed number of items |
| `object/{guid}` returns a **different `objectId` and address** than the one from `suggest` | The object card is generated too, not looked up by GUID |

What **does** work and is genuinely useful:

- **Responses are deterministic.** The same request always returns the same result — so you can write stable tests and build the integration without spending anything.
- **The response shape is real.** Every field, code and nesting level is exactly what a live key returns.

What you **cannot** do in the sandbox: evaluate search quality or suggestion relevance. That needs a live key — and with one, all of this is the real FIAS/GAR data.

## Quick start

Try the API without cloning anything:

```bash
curl -H "Authorization: Bearer ak_sandbox_demo_mockdata_v1" \
     "https://atlorium.com/api/Gar/suggest?query=москва%20тверская&limit=5"
```

| Language | Run | Requires |
|----------|-----|----------|
| [Python](python/) | `pip install -r requirements.txt && python main.py` | Python 3.10+ |
| [TypeScript / Node.js](node/) | `npm install && npm start` | Node.js 20+ |
| [Go](go/) | `go run .` | Go 1.22+ |
| [Java](java/) | `java Main.java` | JDK 17+ (no dependencies) |
| [C#](csharp/) | `dotnet run` | .NET 8+ |
| [PHP](php/) | `php main.php` | PHP 8.1+ |

Pass your own address prefix as the first argument and the index of the picked suggestion as the second:

```bash
python main.py "санкт-петербург невский" 2
```

## Authentication

The key goes in the `Authorization` header:

```
Authorization: Bearer YOUR_KEY
```

| Key | Behaviour |
|-----|-----------|
| `ak_sandbox_demo_mockdata_v1` | **Demo key.** Public, shared by everyone. Returns mocks, charges nothing, needs no account. Responses are deterministic, so you can assert on them in tests. |
| Live key | Real GAR / FIAS data. Get one at [atlorium.com](https://atlorium.com) |

Switching to a live key requires **no code changes** — every example reads an environment variable:

```bash
export ATLORIUM_API_KEY="ak_your_live_key"
```

Every sandbox response carries the header `X-Atlorium-Sandbox: true`, so mock data can never be mistaken for real data.

## Endpoints

Base URL: `https://atlorium.com`

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/Gar/suggest` | **Autocomplete**: fast suggestions by prefix, for the dropdown under an input field |
| `GET` | `/api/Gar/search` | **Full search**: the same, plus OKTMO, OKATO, postal code and region code |
| `GET` | `/api/Gar/object/{guid}` | Address object card by **FIAS GUID** |
| `GET` | `/api/Gar/object/id/{objectId}` | Address object card by numeric `objectId` |
| `GET` | `/api/Gar/hierarchy/{objectId}` | Object hierarchy: region → city → street → building |
| `GET` | `/api/Gar/children/{parentObjectId}` | Child objects: streets of a city, buildings on a street |
| `GET` | `/api/Gar/regions` | List of Russian regions |
| `GET` | `/api/Gar/stats` | Address database statistics |
| `GET` | `/api/Gar/region/{regionCode}/stats` | Statistics for a single region |

### `GET /api/Gar/suggest`

Fast suggestions for autocomplete. Returns the minimum needed to render a dropdown.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `query` | query | string | The address prefix the user is typing, e.g. `москва тверская`. **At least 2 characters**, otherwise `400` |
| `limit` | query | int | How many suggestions to return. Defaults to `7` |

### `GET /api/Gar/search`

Full search. Unlike `suggest`, every result already carries `regionCode`, `postalCode`, `oktmo` and `okato` — no second request needed.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `query` | query | string | Search string, e.g. `Тверская` |
| `limit` | query | int | Max results. Defaults to `10` |

### `GET /api/Gar/object/{guid}`

Object card by FIAS GUID — the second half of an autocomplete flow.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `guid` | path | string (uuid) | **FIAS GUID** of the address object, taken from `objectGuid` of a suggestion or search result |

### `GET /api/Gar/object/id/{objectId}`

The same card, addressed by the internal numeric GAR identifier.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `objectId` | path | int | Numeric `objectId` of the address object |

### `GET /api/Gar/hierarchy/{objectId}`

The path up the tree: from a building to its region. Use it to split an address into components.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `objectId` | path | int | Numeric `objectId` of the address object |

### `GET /api/Gar/children/{parentObjectId}`

Child objects: streets of a given city, buildings on a given street. Useful for cascading "region → city → street" dropdowns.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `parentObjectId` | path | int | `objectId` of the parent object |
| `limit` | query | int | Max children. Defaults to `50` |

### `GET /api/Gar/regions`

List of Russian regions. No parameters.

### `GET /api/Gar/stats`

Address database statistics: the size of the loaded GAR snapshot. No parameters.

### `GET /api/Gar/region/{regionCode}/stats`

Statistics for one region.

| Parameter | In | Type | Description |
|-----------|----|------|-------------|
| `regionCode` | path | string | Russian region code, e.g. `54` (Novosibirsk Oblast), `77` (Moscow) |

## Response fields

Below are the fields taken from **live API responses**. The complete schema for every endpoint is in [Swagger](https://atlorium.com/garAPI) and the [OpenAPI spec](https://atlorium.com/openapi/gar_en-US.json).

Note the asymmetry: `suggest` puts its items in **`items`**, `search` puts them in **`results`**. That is not a typo — they are different responses.

### `suggest`

| Field | Type | Meaning |
|-------|------|---------|
| `query` | string | The original query string |
| `items` | array | Suggestions (see below) |
| `count` | int | How many suggestions were found |
| `elapsedMs` | int | Local database lookup time, ms |

`items[]` element:

| Field | Type | Meaning |
|-------|------|---------|
| `text` | string | Display-ready address line |
| `objectId` | int | Internal numeric GAR identifier |
| `objectGuid` | string (uuid) | **The FIAS code. This is the value you store in your database** |
| `objectType` | string | Object type: building, street, city, … |

### `search`

| Field | Type | Meaning |
|-------|------|---------|
| `query` | string | The original query string |
| `results` | array | Results (see below) |
| `count` | int | How many results were found |
| `elapsedMs` | int | Lookup time, ms |

`results[]` element:

| Field | Type | Meaning |
|-------|------|---------|
| `fullAddress` | string | Full address on one line |
| `objectType` | string | Object type |
| `objectGuid` | string (uuid) | FIAS code |
| `objectId` | int | Numeric GAR identifier |
| `regionCode` | string | Russian region code |
| `postalCode` | string | **Postal code** |
| `oktmo` | string | **OKTMO** — municipal territory classifier code |
| `okato` | string | **OKATO** — administrative territory classifier code |

### `object/{guid}` and `object/id/{objectId}`

| Field | Type | Meaning |
|-------|------|---------|
| `objectId` | int | Numeric GAR identifier |
| `objectGuid` | string (uuid) | **FIAS code** |
| `fullAddress` | string | Canonical full address |
| `objectType` | string | Object type |
| `regionCode` | string | Russian region code |
| `hierarchy` | array | The address split into levels (see below) |
| `parameters` | array | Address codes (see below) |
| `elapsedMs` | int | Lookup time, ms |

`hierarchy[]` element — one level, from region down to building:

| Field | Type | Meaning |
|-------|------|---------|
| `objectId` | int | Identifier of the object at this level |
| `displayName` | string | Name: `Новосибирская обл`, `г Рязань`, `ул ул. Садовый`, `д. 186` |

`parameters[]` element — **this is the "normalized address for your database"**: type/value pairs.

| `typeId` | `typeName` | Example `value` | English |
|----------|------------|-----------------|---------|
| `5` | Почтовый индекс | `328391` | Postal code |
| `6` | ОКТМО | `83736741` | OKTMO |
| `7` | ОКАТО | `51222794` | OKATO |
| `8` | Код ИФНС | `6016` | Tax office code |
| `10` | Код КЛАДР | `74847877663722792` | KLADR code |

### `hierarchy/{objectId}`

| Field | Type | Meaning |
|-------|------|---------|
| `objectId` | int | Identifier of the requested object |
| `path` | array | Path from region to object: `{ objectId, displayName }` elements |
| `elapsedMs` | int | Lookup time, ms |

Note: the object card exposes its breakdown as `hierarchy`, while the dedicated `/hierarchy/{objectId}` endpoint calls it `path`.

## Error handling

| Code | Cause | What to do |
|------|-------|------------|
| `400` | Bad request: empty query, malformed GUID or `objectId` | Check the request parameters |
| `401` | Key missing, expired or invalid | Check the `Authorization` header |
| `402` | Insufficient credit balance | Top up at [atlorium.com](https://atlorium.com) |
| `404` | Address object not found | The GUID is well-formed, but no such object exists in GAR. The database lookup has already run, so the request is billed |
| `429` | Rate limit exceeded | Retry with backoff. For autocomplete, always debounce the input field |
| `500` | Internal error while querying the address database | Retry later. **You are not charged for our failures** |
| `503` | Service temporarily unavailable: scheduled maintenance | Retry later; `message` links to the [status page](https://atlorium.com/status). **Not billed** |

All six examples map these codes to human-readable causes — see the `AtloriumError` class.

## Pricing

**Pay-as-you-go, no subscription** — you pay per executed request. Note: an object lookup is billed even when the object is not found (`404`), because the database query has already run. Malformed input (`400`) and internal errors (`500`) are not billed.

A practical note on autocomplete: **debounce the input field by 250–300 ms** instead of calling `suggest` on every keystroke. Otherwise one typed address turns into a dozen requests instead of two or three.

Current prices: **[atlorium.com/pricing](https://atlorium.com/pricing)**

## FAQ

**What is GAR, and how does it relate to FIAS?** GAR (the State Address Register) replaced FIAS in 2021. The identifiers (GUIDs) stayed the same, so "FIAS code" and "GAR GUID" mean the same thing in practice, and both names are used here.

**Why can't I just store the address as a string?** Because "ул. Лесная", "улица Лесная" and "Лесная ул." are one street and three different strings. Six months in, your database has three customers instead of one, and reconciling with anyone else's data becomes impossible. `objectGuid` fixes this outright: one per object, and it never changes.

**How do I get OKTMO for an address?** Find the object with `suggest` or `search`, then call `/api/Gar/object/{guid}` — OKTMO arrives in the `parameters` array (`typeId: 6`). With `search` it is already in the result, no second call needed.

**How do I get the KLADR code for an address?** Same way — `parameters`, `typeId: 10`. KLADR is a legacy classifier, but plenty of accounting systems and government forms still demand it, so GAR keeps serving it.

**How is `suggest` different from `search`?** `suggest` is for the dropdown: fewest fields, fastest response, items in `items`. `search` is for server-side processing: the same addresses plus OKTMO, OKATO, postal code and region code right in the response, items in `results`.

**Does the data come from the internet?** No. The service runs against a **local copy of the GAR database**, which is why it answers in single-digit milliseconds and does not depend on any third party being up.

**Do I need to sign up to try it?** No. The demo key is public and works without an account — but it returns mocks, not real addresses (see the sandbox section above).

## Other Atlorium APIs

An address rarely lives alone — a phone number, an email and company details usually sit right next to it. The same account and the same key also give you:

- [Address standardization](https://github.com/atlorium-api/address-standardization-api-client) — parse a string into components, quality score
- [Weather data](https://github.com/atlorium-api/weather-api-client) — current conditions by coordinates
- [Phone validation](https://github.com/atlorium-api/phone-validation-api-client) — format, line type, range operator
- [Email verification](https://github.com/atlorium-api/email-verification-api-client) — syntax, MX records, disposable addresses
- [Forward geocoding](https://github.com/atlorium-api/geocoding-api-client) — coordinates from an address, batch included
- [Reverse geocoding](https://github.com/atlorium-api/reverse-geocoding-api-client) — address from coordinates, neighbours and landmarks

Full catalogue: [atlorium.com](https://atlorium.com)

## Links

- **API reference (Swagger):** [atlorium.com/garAPI](https://atlorium.com/garAPI)
- **Service description:** [atlorium.com/garDescription](https://atlorium.com/garDescription)
- **Web interface:** [atlorium.com/garGUI](https://atlorium.com/garGUI)
- **OpenAPI spec:** [gar_en-US.json](https://atlorium.com/openapi/gar_en-US.json)
- **Support:** support@atlorium.com

## License

[MIT](LICENSE)
