# Friendbot Service for the Stellar Test Network

[![Apache 2.0 licensed](https://img.shields.io/badge/license-apache%202.0-blue.svg)](LICENSE)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/stellar/quickstart)

Stellar's native asset faucet.

Friendbot helps users of the Stellar testnet by exposing a REST endpoint that creates & funds new accounts.

> [!WARNING]
>
> Merges of pull requests to the main branch deploy to testnet immediately.

> [!TIP]
> 
> Friendbot for [Testnet] is hosted at https://friendbot.stellar.org.
> Friendbot for [Futurenet] is hosted at https://friendbot-futurenet.stellar.org.
> Friendbot in [Quickstart] is available at http://localhost:8000/friendbot.

[Testnet]: https://developers.stellar.org/docs/networks
[Futurenet]: https://developers.stellar.org/docs/networks
[Quickstart]: https://github.com/stellar/quickstart

## API Usage

Friendbot exposes a simple REST API with a single endpoint that accepts both GET and POST requests.

### Endpoint

```
GET|POST /
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `addr` | string | Yes | The Stellar account address to fund |

### Examples

#### Using cURL

**GET request**:
```bash
curl "http://localhost:8000/?addr=GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"
```

**POST request**:
```bash
curl -X POST "http://localhost:8000/" \
  -d "addr=GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z"
```

#### Using JavaScript

```javascript
// GET request
const response = await fetch('http://localhost:8000/?addr=GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z');
const transaction = await response.json();

// POST request
const response = await fetch('http://localhost:8000/', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/x-www-form-urlencoded',
  },
  body: 'addr=GDJIN6W6PLTPKLLM57UW65ZH4BITUXUMYQHIMAZFYXF45PZVAWDBI77Z'
});
const transaction = await response.json();
```

### Response

On success, the API returns a Stellar transaction object:

```json
{
  "envelope_xdr": "AAAAAgAAAAD4Az3jKU6lbzq/L5HG9/GzBT+FYusOz71oyYMbZkP+GAAAAGQAAAAAAAAAAgAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAABAAAAAPXQ8gjyrVHa47a6JDPkVHwPPDKxNRE2QBcamA4JvlOGAAAAAAAAAADShvreeub1LWzv6W93J+BROl6MxA6GAyXFy86/NQWGFAAAABdIdugAAAAAAAAAAAJmQ/4YAAAAQDRLEljDVYALnTk9mDceQEd5PrjQyE3LUAjstIyTWH5t/TP909F66TgEfBFKMxSKF6fka7ZuPcSs40ix4AomEgoJvlOGAAAAQPSGs88OwXubz7UT6nFhvhF47EQfaOsmiIsOkjgzUrmBoypJQTmMMbgeix0kdbfHqS75+iefJpdXLNFDreGnxgE=",
  "successful": true,
  "hash": "c4c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8c0d8",
  "ledger": 12345678,
  "created_at": "2024-01-01T00:00:00Z"
}
```

### Error Responses

The API returns appropriate HTTP status codes and error details:

- **400 Bad Request**: Invalid account address or account already funded
- **404 Not Found**: Account does not exist (for certain operations)
- **500 Internal Server Error**: Server-side error

Example error response:
```json
{
  "type": "https://stellar.org/horizon-errors/bad_request",
  "title": "Bad Request",
  "status": 400,
  "detail": "Account already funded"
}
```

## Running Friendbot

### Docker

Run Friendbot using the pre-built Docker image:

```
docker run \
  --platform linux/amd64 \
  -v $PWD/friendbot.cfg:/friendbot.cfg \
  -p 8004:8004 \
  stellar/friendbot:<FULL-GIT-SHA>
```

See below for how to configure the `friendbot.cfg` file that needs to be mounted.

The service will be available at `http://localhost:8004`.

🔗 **[View on Docker Hub](https://hub.docker.com/r/stellar/friendbot)**

### Source

1. **Build**:
   ```
   go build -o friendbot .
   ```

2. Configure a `friendbot.cfg` file.

3. **Run**:
   ```
   ./friendbot --conf=friendbot.cfg
   ```

### Configuration

The service uses a TOML configuration file. Here are the key settings:

| Setting | Description | Default |
|---------|-------------|---------|
| `port` | Port to listen on | `8000` |
| `friendbot_secret` | Secret key for the friendbot account | Required |
| `network_passphrase` | Stellar network passphrase | `"Test SDF Network ; September 2015"` |
| `horizon_url` | Horizon server URL | `"https://horizon-testnet.stellar.org"` |
| `starting_balance` | Initial native balance for new accounts | `"10000.00"` |
| `num_minions` | Number of minion accounts for parallel processing | `1000` |
| `base_fee` | Base fee for transactions | `100000` |
| `minion_batch_size` | Batch size for minion operations | `50` |
| `submit_tx_retries_allowed` | Number of retry attempts for failed transactions | `5` |


## Development

See [CONTRIBUTING.md].

[CONTRIBUTING.md]: ./CONTRIBUTING.md

### Checking Build

```
go build ./...
```

### Running Tests

```
go test ./...
```
