import 'package:flutter_secure_storage/flutter_secure_storage.dart';

/// The only place the Kratos `session_token` is persisted (Keychain /
/// Keystore). A copy is cached in memory to avoid a platform-channel hop per
/// API request. The token is never logged.
abstract interface class TokenStore {
  Future<String?> read();
  Future<void> write(String token);
  Future<void> clear();
}

class SecureTokenStore implements TokenStore {
  SecureTokenStore({FlutterSecureStorage? storage})
    : _storage =
          storage ??
          const FlutterSecureStorage(
            // security §6.6: first_unlock_this_device on iOS; Keystore-backed
            // encrypted storage on Android.
            iOptions: IOSOptions(
              accessibility: KeychainAccessibility.first_unlock_this_device,
            ),
          );

  static const _key = 'kratos_session_token';
  final FlutterSecureStorage _storage;
  String? _cache;
  bool _loaded = false;

  @override
  Future<String?> read() async {
    if (_loaded) return _cache;
    _cache = await _storage.read(key: _key);
    _loaded = true;
    return _cache;
  }

  @override
  Future<void> write(String token) async {
    await _storage.write(key: _key, value: token);
    _cache = token;
    _loaded = true;
  }

  @override
  Future<void> clear() async {
    _cache = null;
    _loaded = true;
    await _storage.delete(key: _key);
  }
}

/// In-memory store for tests and the plain-Dart integration test.
class InMemoryTokenStore implements TokenStore {
  InMemoryTokenStore([this._token]);

  String? _token;

  @override
  Future<String?> read() async => _token;

  @override
  Future<void> write(String token) async => _token = token;

  @override
  Future<void> clear() async => _token = null;
}
