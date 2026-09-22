import { createCipheriv, generateKeyPairSync, randomBytes } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { BackendContext } from "./backend";

/**
 * Deployment-owned material that makes a worker's backend report
 * `available: true` for the dedicated personal-star connection.
 *
 * `available` is a deployment fact, not a feature flag: the GitHub provider
 * only builds the dedicated star runtime when the OAuth App settings are
 * complete AND the data directory holds a bootstrapped deployment identity, an
 * existing master key, and a readable `deployment-secrets.sqlite` carrying the
 * protected client-secret reference. This module writes exactly that material
 * before the backend is restarted, mirroring
 * `apps/backend/cmd/deployment-secret-seed` (which uses the real
 * `internal/secrets` packages) byte-for-byte:
 *
 * - `master.key` — 32 random bytes, mode 0600; the backend creates it on
 *   ordinary startup, so an existing key is reused rather than replaced.
 * - `deployment/installation_id` (16 random bytes as lowercase hex),
 *   `deployment/installation_key` (Ed25519 seed||public, mode 0600) and
 *   `deployment/installation_key.pub`.
 * - `deployment-secrets.sqlite` with one `deployment_secret_refs` row for
 *   `kandev-runtime:github-personal-star:client-secret:<version>`, holding the
 *   canonical AES-256-GCM JCS sealed envelope encrypted under that master key.
 *
 * The fixtures cannot shell out to the Go command: the E2E shard image ships no
 * Go toolchain, and specs run against the prebuilt backend artifact. The
 * mirroring is therefore asserted from the other side — a spec that seeds a
 * capable deployment fails loudly if the backend reports `available: false`.
 */

export const PERSONAL_STAR_CLIENT_ID = "e2e-personal-star-client-id";
export const PERSONAL_STAR_CLIENT_SECRET = "e2e-personal-star-client-secret";
const PERSONAL_STAR_CLIENT_SECRET_REF_PREFIX = "kandev-runtime:github-personal-star:client-secret:";
const PERSONAL_STAR_CALLBACK_PATH = "/api/v1/github/user/starred/oauth/callback";

/** Mirrors internal/secrets constants; the reader rejects any other spelling. */
const MASTER_KEY_FILE = "master.key";
const MASTER_KEY_BYTES = 32;
const MASTER_KEY_ID = "legacy-master-key-v1";
const STORE_FILE = "deployment-secrets.sqlite";
const SECRET_CLASS = "github-personal-star-client";
const ROTATION_ACTIVE = "active";
const IDENTITY_DIR = "deployment";
const INSTALLATION_ID_FILE = "installation_id";
const INSTALLATION_ID_BYTES = 16;
const PRIVATE_KEY_FILE = "installation_key";
const PUBLIC_KEY_FILE = "installation_key.pub";

export type PersonalStarDeploymentSeed = {
  /**
   * Environment overrides that expose the seeded deployment. Apply them with
   * `backend.restart(seed.env)` — the material is on disk before that process
   * reads it.
   */
  env: Record<string, string>;
  /** Absolute path of the seeded data directory. */
  dataDir: string;
};

/**
 * Seeds the deployment-owned material for a worker backend and returns the
 * environment that exposes it. Idempotent: an existing identity, master key,
 * and reference row are reused.
 */
export function seedPersonalStarDeployment(
  backend: BackendContext,
  options: { clientSecret?: string; version?: number } = {},
): PersonalStarDeploymentSeed {
  const version = options.version ?? 1;
  const clientSecret = options.clientSecret ?? PERSONAL_STAR_CLIENT_SECRET;
  const reference = `${PERSONAL_STAR_CLIENT_SECRET_REF_PREFIX}${version}`;
  const dataDir = backend.dataDir;
  fs.mkdirSync(dataDir, { recursive: true });

  const masterKey = loadOrCreateMasterKey(dataDir);
  const installationID = ensureDeploymentIdentity(dataDir);
  writeDeploymentSecretStore(dataDir, {
    installationID,
    masterKey,
    reference,
    version,
    clientSecret,
  });

  return {
    dataDir,
    env: {
      KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_ID: PERSONAL_STAR_CLIENT_ID,
      KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CALLBACK_URL: `${backend.baseUrl}${PERSONAL_STAR_CALLBACK_PATH}`,
      KANDEV_GITHUB_PERSONAL_STAR_OAUTH_CLIENT_SECRET_REF: reference,
    },
  };
}

/** Reuses the backend's master key, creating one the way the backend would. */
function loadOrCreateMasterKey(dataDir: string): Buffer {
  const keyPath = path.join(dataDir, MASTER_KEY_FILE);
  if (fs.existsSync(keyPath)) {
    const key = fs.readFileSync(keyPath);
    if (key.length !== MASTER_KEY_BYTES) {
      throw new Error(`${keyPath} is not a ${MASTER_KEY_BYTES}-byte master key`);
    }
    return key;
  }
  const key = randomBytes(MASTER_KEY_BYTES);
  fs.writeFileSync(keyPath, key, { mode: 0o600 });
  return key;
}

/** Bootstraps the identity exactly once, mirroring BootstrapDeploymentIdentity. */
function ensureDeploymentIdentity(dataDir: string): string {
  const dir = path.join(dataDir, IDENTITY_DIR);
  const installationIDPath = path.join(dir, INSTALLATION_ID_FILE);
  const publicKeyPath = path.join(dir, PUBLIC_KEY_FILE);
  const privateKeyPath = path.join(dir, PRIVATE_KEY_FILE);
  const paths = [installationIDPath, privateKeyPath, publicKeyPath];
  const present = paths.filter((candidate) => fs.existsSync(candidate));
  if (present.length === paths.length) {
    return fs.readFileSync(installationIDPath, "utf8").trim();
  }
  if (present.length > 0) {
    // Bootstrap never repairs or overwrites deployment-owned trust material.
    throw new Error(`incomplete deployment identity in ${dir}: ${present.join(", ")}`);
  }

  const { privateKey, publicKey } = rawEd25519Keys();
  const installationID = randomBytes(INSTALLATION_ID_BYTES).toString("hex");
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  fs.writeFileSync(installationIDPath, `${installationID}\n`, { mode: 0o644 });
  fs.writeFileSync(privateKeyPath, privateKey, { mode: 0o600 });
  fs.writeFileSync(publicKeyPath, publicKey, { mode: 0o644 });
  return installationID;
}

/**
 * Go's Ed25519 private key is the 32-byte seed followed by the 32-byte public
 * key, so the DER wrappers are unwrapped to the same raw layout the operator
 * bootstrap writes.
 */
function rawEd25519Keys(): { privateKey: Buffer; publicKey: Buffer } {
  const { privateKey: derPrivate, publicKey: derPublic } = generateKeyPairSync("ed25519");
  const pkcs8 = derPrivate.export({ format: "der", type: "pkcs8" });
  const spki = derPublic.export({ format: "der", type: "spki" });
  if (pkcs8.length !== 48 || spki.length !== 44) {
    throw new Error("unexpected Ed25519 DER lengths while seeding the deployment identity");
  }
  const seed = Buffer.from(pkcs8.subarray(16));
  const publicKey = Buffer.from(spki.subarray(12));
  return { privateKey: Buffer.concat([seed, publicKey]), publicKey };
}

/** Writes the immutable reference row, keeping an already-seeded row. */
function writeDeploymentSecretStore(
  dataDir: string,
  seed: {
    installationID: string;
    masterKey: Buffer;
    reference: string;
    version: number;
    clientSecret: string;
  },
): void {
  const db = new DatabaseSync(path.join(dataDir, STORE_FILE));
  try {
    // The backend opens the store read-only; the rollback journal keeps the
    // single file self-contained.
    db.exec("PRAGMA journal_mode = DELETE");
    db.exec(`
      CREATE TABLE IF NOT EXISTS deployment_secret_refs (
        reference TEXT PRIMARY KEY,
        secret_class TEXT NOT NULL CHECK (secret_class = '${SECRET_CLASS}'),
        installation_id TEXT NOT NULL CHECK (installation_id <> ''),
        version BIGINT NOT NULL CHECK (version >= 1),
        sealed_envelope TEXT NOT NULL CHECK (sealed_envelope <> ''),
        rotation_state TEXT NOT NULL CHECK (rotation_state IN ('${ROTATION_ACTIVE}', 'revoked')),
        created_at TEXT NOT NULL,
        revoked_at TEXT
      );
      CREATE UNIQUE INDEX IF NOT EXISTS deployment_secret_refs_version
        ON deployment_secret_refs (secret_class, installation_id, version);
    `);
    db.prepare(
      `INSERT INTO deployment_secret_refs
        (reference, secret_class, installation_id, version, sealed_envelope, rotation_state, created_at, revoked_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, NULL)
       ON CONFLICT (reference) DO NOTHING`,
    ).run(
      seed.reference,
      SECRET_CLASS,
      seed.installationID,
      seed.version,
      sealClientSecretEnvelope(seed.masterKey, seed.clientSecret),
      ROTATION_ACTIVE,
      new Date().toISOString(),
    );
  } finally {
    db.close();
  }
}

/**
 * Renders the canonical AES-256-GCM JCS sealed envelope. Keys are sorted per
 * RFC 8785 and the 16-byte GCM tag is appended to the ciphertext, matching
 * internal/secrets throughout.
 */
function sealClientSecretEnvelope(masterKey: Buffer, clientSecret: string): string {
  const nonce = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", masterKey, nonce);
  const ciphertext = Buffer.concat([
    cipher.update(clientSecret, "utf8"),
    cipher.final(),
    cipher.getAuthTag(),
  ]);
  // Strict RFC 4648 unpadded standard Base64, as the reader requires.
  return (
    `{"algorithm":"AES-256-GCM",` +
    `"ciphertext_b64":"${ciphertext.toString("base64").replace(/=+$/u, "")}",` +
    `"key_id":"${MASTER_KEY_ID}",` +
    `"nonce_b64":"${nonce.toString("base64").replace(/=+$/u, "")}",` +
    `"version":1}`
  );
}
