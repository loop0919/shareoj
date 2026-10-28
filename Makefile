.PHONY: dev db-up db-down

# Reuse the pinned toolchain even when direnv has not been enabled.
# dev.mjs reads the root .env itself; Bun's own loader would expand $ in its values.
dev:
	@nix develop . --command bun --no-env-file scripts/dev.mjs

db-up:
	@nix develop . --command bun --no-env-file scripts/db.mjs

db-down:
	@nix develop . --command bun --no-env-file scripts/db.mjs stop
