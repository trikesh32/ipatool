# Linux Store proof helper

This optional Linux helper restores the Store AMD machine proof used by native
App Store authentication and commerce requests. It generates fresh `X-Apple-AMD`
and `X-Apple-AMD-M` locally with Apple's Android CoreADI libraries, using Store
machine scope **-1**. AuthKit/GrandSlam anisette (scope -2) is a different protocol.
No Mac, remote anisette service, or Apple ID credentials are needed by the helper.

The main CLI retains its macOS implementation and its existing non-Darwin behavior
unless `IPATOOL_STORE_PROOF_HELPER` is set. With this variable set on Linux, it uses
Configurator 2.21, obtains fresh proof for allowlisted requests, and handles an
`X-Apple-AMD-Action: SP` response by provisioning and retrying the identical signed
request once. Repeated SP, unsupported challenges, and provisioning failures are
reported instead of retried indefinitely.

## Native Linux build

Requires Go 1.26.5 or newer, a C compiler, and glibc on amd64 or arm64.
From the repository root:

```sh
mkdir -p bin
go build -o bin/ipatool .
(cd tools/store-proof && go build -o ../../bin/ipatool-store-proof .)
./bin/ipatool-store-proof --setup
export IPATOOL_STORE_PROOF_HELPER="$PWD/bin/ipatool-store-proof"
./bin/ipatool auth login
./bin/ipatool list-purchases --platform iphone --max-results 1
```

`--setup` downloads Apple's public Apple Music APK, extracts the two libraries,
checks their pinned SHA-256 hashes, and creates a random local machine identity.
It does not sign in. An Apple APK update that changes these libraries fails closed
until the new libraries have been reviewed and their hashes updated.

Enter the Apple ID password and 2FA code at the CLI prompts. On headless Linux,
ipatool may also request a separate passphrase for its local encrypted keychain.
For downloads, use an app already owned by the account; new-license acquisition
has not been verified by this change.

## Docker / Colima

```sh
docker build -f Dockerfile.store-proof -t ipatool-store-proof .
docker volume create ipatool-state
docker run --rm -v ipatool-state:/root \
  --entrypoint /usr/local/bin/ipatool-store-proof ipatool-store-proof --setup
docker run --rm -it -v ipatool-state:/root ipatool-store-proof auth login
docker run --rm -it -v ipatool-state:/root ipatool-store-proof \
  list-purchases --platform iphone --max-results 1
```

On Apple Silicon, add `--platform linux/amd64` to both build and run to test x64
through Colima's Rosetta support. The proof generation itself runs inside Linux.
The initial ipatool login may download its existing SAP framework assets as usual.

## State and protocol

The helper stores its identity and ADI state in
`$XDG_CONFIG_HOME/ipatool/store-proof`, or `~/.config/ipatool/store-proof`.
`IPATOOL_STORE_PROOF_DIR` overrides this with an absolute directory. Keep this state
private and persistent. It is separate from ipatool's account session/keychain.

`--headers` writes one JSON object with `provisioned` and, when ready, `headers`.
An unprovisioned machine returns `{"provisioned":false}`. `--provision` reads one
JSON object containing only `challenge` and `guid` from stdin and returns fresh
headers after completing Apple's Store provisioning exchange. The helper receives
no username, password, 2FA code, cookies, or Store session token.

The CLI invokes an absolute executable path without a shell, bounds its output,
enforces a timeout, and accepts exactly the two Store proof headers. Provisioning
uses a fixed HTTPS Apple endpoint and never follows redirects. Helper operations
are serialized with a filesystem lock. Apple's native libraries run in a separate
process from the CLI.

## Tests and licensing

```sh
go generate ./...
go test ./internal/store ./pkg/http ./pkg/appstore
(cd tools/store-proof && go test ./...)
```

The helper is a separate Go module under LGPLv2. Its ADI wrapper and ELF loader
derive from hpidcock/ap and Dadoum/Provision; see [NOTICE](NOTICE) and
[COPYING-LGPLv2](COPYING-LGPLv2). Preserve those files and provide the corresponding
helper source when sharing binaries. Apple libraries are downloaded locally and
are not included in this repository or image.

Live validation so far: one account on Linux amd64 in Colima/Rosetta completed
password + 2FA login, a new process read the saved session, owned iPhone apps were
listed, and Overcast 2026.10.1 (build 1182) downloaded successfully (55.3 MB;
ZIP CRC, bundle ID and iPhoneOS platform checked). A fresh helper identity also
completed automatic SP provisioning, and the final packaged helper downloaded
the same app using the saved account session. This is not a claim that all
Apple accounts, challenges, platforms, or purchase flows work.
