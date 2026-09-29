# bdash

A terminal dashboard for the [bd (beads)](https://github.com/steveyegge/beads) issue tracker, built with Bubble Tea v2.

Work in progress: nothing usable yet.

## Development

Requires Go (see `go.mod`), [just](https://github.com/casey/just) and git.

```sh
just ci            # tidy, format, lint, vet, build, full tests
just test-short    # skip the real-bd integration tests
just golden-update # rewrite golden files after an intended rendering change
```

Integration tests run against pinned bd releases installed into `.cache/bd/<version>/` by `scripts/install-bd.sh` (checksum-verified). Point `BDASH_TEST_BD_1_2_2` / `BDASH_TEST_BD_1_3_0` or `BDASH_TEST_BD_DIR` at other binaries to override.

## License

MIT, see [LICENSE](LICENSE).
