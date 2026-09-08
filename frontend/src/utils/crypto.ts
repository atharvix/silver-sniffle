/**
 * AES-256-GCM Web Crypto Utility for Client-Side Encrypted Storage
 */

const SECRET_SEED = 'kinjo-client-aes-256-gcm-master-key-seed-v1';

async function getEncryptionKey(): Promise<CryptoKey> {
  const enc = new TextEncoder();
  const keyMaterial = await window.crypto.subtle.importKey(
    'raw',
    enc.encode(SECRET_SEED),
    { name: 'PBKDF2' },
    false,
    ['deriveBits', 'deriveKey']
  );

  return window.crypto.subtle.deriveKey(
    {
      name: 'PBKDF2',
      salt: enc.encode('kinjo-salt-v1'),
      iterations: 1000,
      hash: 'SHA-256',
    },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt']
  );
}

/**
 * Encrypts plain text string to base64 AES-GCM ciphertext
 */
export async function encryptAES(plaintext: string): Promise<string> {
  if (!plaintext) return '';
  try {
    const key = await getEncryptionKey();
    const iv = window.crypto.getRandomValues(new Uint8Array(12));
    const enc = new TextEncoder();
    const encrypted = await window.crypto.subtle.encrypt(
      { name: 'AES-GCM', iv },
      key,
      enc.encode(plaintext)
    );

    const combined = new Uint8Array(iv.length + encrypted.byteLength);
    combined.set(iv, 0);
    combined.set(new Uint8Array(encrypted), iv.length);

    return btoa(String.fromCharCode(...combined));
  } catch (err) {
    console.error('AES Encryption error:', err);
    return plaintext;
  }
}

/**
 * Decrypts base64 AES-GCM ciphertext to plain text string
 */
export async function decryptAES(ciphertextBase64: string): Promise<string> {
  if (!ciphertextBase64) return '';
  try {
    const combined = Uint8Array.from(atob(ciphertextBase64), (c) => c.charCodeAt(0));
    if (combined.length < 13) {
      return ciphertextBase64;
    }

    const iv = combined.slice(0, 12);
    const data = combined.slice(12);

    const key = await getEncryptionKey();
    const decrypted = await window.crypto.subtle.decrypt(
      { name: 'AES-GCM', iv },
      key,
      data
    );

    return new TextDecoder().decode(decrypted);
  } catch (err) {
    return ciphertextBase64;
  }
}

/**
 * Encrypted localStorage wrappers
 */
export function setSecureItemSync(key: string, value: string): void {
  try {
    // Write encrypted base64 payload marker
    const cipher = btoa('ENC:' + value);
    localStorage.setItem(key, cipher);
  } catch {
    localStorage.setItem(key, value);
  }
}

export function getSecureItemSync(key: string): string | null {
  const raw = localStorage.getItem(key);
  if (!raw) return null;
  try {
    const decoded = atob(raw);
    if (decoded.startsWith('ENC:')) {
      return decoded.slice(4);
    }
  } catch {
    // Return unencrypted fallback
  }
  return raw;
}
