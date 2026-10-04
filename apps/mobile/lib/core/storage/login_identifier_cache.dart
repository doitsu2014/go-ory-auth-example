import 'dart:convert';
import 'dart:math';

import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';

/// Remembers the pseudonym a sign-in succeeded with, so the next sign-in
/// with the same email / phone skips the resolver call (DD-18). Entries are
/// keyed by `HMAC-SHA256(install secret, "<type>:<normalised value>")`: the
/// address itself is never stored, and the key cannot be recomputed from a
/// guessed address without the per-install secret (SEC-C13).
abstract interface class LoginIdentifierCache {
  /// The cached pseudonym, or null. A value that is not a pseudonym is
  /// evicted and reported as absent.
  Future<String?> read(LoginInput input);
  Future<void> write(LoginInput input, String identifier);
  Future<void> remove(LoginInput input);

  /// Removes every cached `login_id.*` entry (sign-out).
  Future<void> clear();
}

/// Prefix of every cache entry key.
const loginIdentifierCacheKeyPrefix = 'login_id.';

/// Storage key of the per-install HMAC secret (not a `login_id.*` entry, so
/// [LoginIdentifierCache.clear] keeps it).
const loginIdentifierCacheSecretKey = 'login_cache_secret';

/// Storage key for [input] under [secret]: `login_id.<hmac-sha256 hex>`.
String loginIdentifierCacheKey(LoginInput input, List<int> secret) {
  final mac = Hmac(
    sha256,
    secret,
  ).convert(utf8.encode('${input.type.wire}:${input.normalised}'));
  return '$loginIdentifierCacheKeyPrefix$mac';
}

/// Cache logic over a string key-value store: secret handling, keying,
/// validation and clearing. Subclasses provide the store.
abstract class KeyValueLoginIdentifierCache implements LoginIdentifierCache {
  static const _secretLength = 32;

  Future<String?> readValue(String key);
  Future<void> writeValue(String key, String value);
  Future<void> deleteValue(String key);
  Future<Iterable<String>> keys();

  Future<List<int>>? _secret;

  /// The per-install secret: read once, generated (32 random bytes) and
  /// stored when missing or malformed. A failed load is retried next time.
  Future<List<int>> _secretBytes() => _secret ??= () async {
    try {
      return await _loadSecret();
    } on Object {
      _secret = null;
      rethrow;
    }
  }();

  Future<List<int>> _loadSecret() async {
    final stored = await readValue(loginIdentifierCacheSecretKey);
    if (stored != null) {
      try {
        final bytes = base64Decode(stored);
        if (bytes.length == _secretLength) return bytes;
      } on FormatException {
        // Regenerated below; old entries become unreachable.
      }
    }
    final random = Random.secure();
    final bytes = List<int>.generate(_secretLength, (_) => random.nextInt(256));
    await writeValue(loginIdentifierCacheSecretKey, base64Encode(bytes));
    return bytes;
  }

  /// The storage key for [input] (loads the secret first).
  @visibleForTesting
  Future<String> keyFor(LoginInput input) async =>
      loginIdentifierCacheKey(input, await _secretBytes());

  @override
  Future<String?> read(LoginInput input) async {
    final key = await keyFor(input);
    final value = await readValue(key);
    if (value == null) return null;
    if (!loginPseudonymPattern.hasMatch(value)) {
      await deleteValue(key);
      return null;
    }
    return value;
  }

  @override
  Future<void> write(LoginInput input, String identifier) async {
    if (!loginPseudonymPattern.hasMatch(identifier)) return;
    await writeValue(await keyFor(input), identifier);
  }

  @override
  Future<void> remove(LoginInput input) async =>
      await deleteValue(await keyFor(input));

  @override
  Future<void> clear() async {
    final entries = (await keys())
        .where((k) => k.startsWith(loginIdentifierCacheKeyPrefix))
        .toList();
    for (final k in entries) {
      await deleteValue(k);
    }
  }
}

class SecureLoginIdentifierCache extends KeyValueLoginIdentifierCache {
  SecureLoginIdentifierCache({FlutterSecureStorage? storage})
    : _storage =
          storage ??
          const FlutterSecureStorage(
            iOptions: IOSOptions(
              accessibility: KeychainAccessibility.first_unlock_this_device,
            ),
          );

  final FlutterSecureStorage _storage;

  @override
  Future<String?> readValue(String key) => _storage.read(key: key);

  @override
  Future<void> writeValue(String key, String value) =>
      _storage.write(key: key, value: value);

  @override
  Future<void> deleteValue(String key) => _storage.delete(key: key);

  @override
  Future<Iterable<String>> keys() async => (await _storage.readAll()).keys;
}

/// In-memory cache for tests and the plain-Dart integration test.
class InMemoryLoginIdentifierCache extends KeyValueLoginIdentifierCache {
  /// Everything stored, including the secret under
  /// [loginIdentifierCacheSecretKey].
  final Map<String, String> entries = {};

  @override
  Future<String?> readValue(String key) async => entries[key];

  @override
  Future<void> writeValue(String key, String value) async =>
      entries[key] = value;

  @override
  Future<void> deleteValue(String key) async => entries.remove(key);

  @override
  Future<Iterable<String>> keys() async => entries.keys.toList();
}
