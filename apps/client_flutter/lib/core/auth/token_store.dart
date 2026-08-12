import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class AuthSession {
  const AuthSession({
    required this.accessToken,
    required this.accessExpiresAt,
    required this.refreshToken,
    required this.refreshExpiresAt,
    required this.userId,
    required this.displayName,
    required this.role,
  });

  factory AuthSession.fromJson(Map<String, dynamic> json) => AuthSession(
    accessToken: json['accessToken'] as String,
    accessExpiresAt: DateTime.parse(json['accessTokenExpiresAt'] as String),
    refreshToken: json['refreshToken'] as String,
    refreshExpiresAt: DateTime.parse(json['refreshTokenExpiresAt'] as String),
    userId: (json['user'] as Map<String, dynamic>)['id'] as String,
    displayName: (json['user'] as Map<String, dynamic>)['displayName'] as String,
    role: (json['user'] as Map<String, dynamic>)['role'] as String,
  );

  final String accessToken;
  final DateTime accessExpiresAt;
  final String refreshToken;
  final DateTime refreshExpiresAt;
  final String userId;
  final String displayName;
  final String role;

  Map<String, dynamic> toJson() => <String, dynamic>{
    'accessToken': accessToken,
    'accessTokenExpiresAt': accessExpiresAt.toIso8601String(),
    'refreshToken': refreshToken,
    'refreshTokenExpiresAt': refreshExpiresAt.toIso8601String(),
    'user': <String, dynamic>{'id': userId, 'displayName': displayName, 'role': role},
  };
}

class TokenStore {
  TokenStore([FlutterSecureStorage? storage]) : _storage = storage ?? const FlutterSecureStorage();

  static const _sessionKey = 'auth_session_v1';
  static const _deviceKey = 'device_id_v1';
  final FlutterSecureStorage _storage;

  Future<AuthSession?> read() async {
    final value = await _storage.read(key: _sessionKey);
    if (value == null) return null;
    try { return AuthSession.fromJson(jsonDecode(value) as Map<String, dynamic>); } on FormatException { await clear(); return null; }
  }

  Future<void> write(AuthSession session) => _storage.write(key: _sessionKey, value: jsonEncode(session.toJson()));
  Future<void> clear() => _storage.delete(key: _sessionKey);
  Future<String> deviceId() async {
    final existing = await _storage.read(key: _deviceKey);
    if (existing != null) return existing;
    final value = '${DateTime.now().microsecondsSinceEpoch}-${identityHashCode(this)}';
    await _storage.write(key: _deviceKey, value: value);
    return value;
  }
}
