import 'package:flutter_test/flutter_test.dart';
import 'package:go_ory_auth_mobile/core/identity/login_input.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_models.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';

import '../helpers/helpers.dart';

void main() {
  group('KratosIdentity.loginId / loginVerified', () {
    KratosIdentity identity(Json traits, List<Json> addresses) =>
        KratosIdentity.fromJson({
          'id': 'id-1',
          'schema_id': 'customer',
          'traits': traits,
          'verifiable_addresses': addresses,
        });

    test('traits.login_id is the login id; verified by its address', () {
      final i = identity(
        {'login_id': pseudonym},
        [
          {'value': pseudonym, 'verified': true, 'via': 'email'},
        ],
      );
      expect(i.loginId, pseudonym);
      expect(i.loginVerified, isTrue);
      expect(i.emailVerified, isTrue, reason: 'kept as an alias');
    });

    test('legacy traits.email is used when login_id is absent', () {
      final i = identity(
        {'email': 'an@example.com'},
        [
          {'value': 'an@example.com', 'verified': false, 'via': 'email'},
        ],
      );
      expect(i.loginId, 'an@example.com');
      expect(i.loginVerified, isFalse);
    });

    test('a verified address for another value does not count', () {
      final i = identity(
        {'login_id': pseudonym},
        [
          {'value': 'other@login.invalid', 'verified': true, 'via': 'email'},
        ],
      );
      expect(i.loginVerified, isFalse);
      expect(identity(const {}, const []).loginVerified, isFalse);
    });
  });

  group('Me.fromJson', () {
    Json base(Json extra) => {
      'id': '6f1c2c1e-3f43-4a77-9a55-0f7d1d1b2a10',
      'email_verified': true,
      'locale': 'vi-VN',
      'created_at': '2026-01-01T00:00:00Z',
      ...extra,
    };

    test('phone login: login {type, value}, no email', () {
      final me = Me.fromJson(
        base({
          'login': {'type': 'phone', 'value': '+84901234567'},
        }),
      );
      expect(me.login.type, LoginType.phone);
      expect(me.login.value, '+84901234567');
      expect(me.email, isNull);
      expect(me.emailVerified, isTrue);
    });

    test('email login: login + deprecated email', () {
      final me = Me.fromJson(
        base({
          'login': {'type': 'email', 'value': 'an@example.com'},
          'email': 'an@example.com',
        }),
      );
      expect(me.login.type, LoginType.email);
      expect(me.login.value, 'an@example.com');
      expect(me.email, 'an@example.com');
    });

    test('legacy server without login falls back to email', () {
      final me = Me.fromJson(base({'email': 'an@example.com'}));
      expect(me.login.type, LoginType.email);
      expect(me.login.value, 'an@example.com');
    });
  });
}
