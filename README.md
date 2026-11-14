# mtools-sdk

Official open-source SDK, packaging CLI, bundle verifier, and Sage Garden UI components for building **mtools** plugins.

## Packages

- `github.com/seiortech/mtools-sdk/pluginapi` — Manifest, declarative `surfaces.json` schema, and JSON-RPC 2.0 stdio server (`pluginapi.Serve`). Zero UI/CGO dependencies.
- `github.com/seiortech/mtools-sdk/bundle` — Pure-Go signed `.zip` bundle inspector and Ed25519 signature verifier (`bundle.InspectBundle`).
- `github.com/seiortech/mtools-sdk/cmd/mtools-plugin` — CLI tool (`keygen`, `pack`, `verify`) for signing and packaging plugins.
- `github.com/seiortech/mtools-sdk/ui` — Separate Go sub-module providing the **Sage Garden** `theme` and reusable `github.com/egoist/mygo/ui` `components`.

## Quickstart

Install the packaging CLI:

```sh
go install github.com/seiortech/mtools-sdk/cmd/mtools-plugin@latest
```

Import `pluginapi` in your plugin sidecar:

```sh
go get github.com/seiortech/mtools-sdk
```

See `examples/plugins/echo` for a complete standalone plugin example.
