import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/app.dart';

/// Entry point. Configure with
/// `flutter run --dart-define-from-file=env/local.json`.
void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(
    // Repositories already map failures; automatic provider retry would
    // repeat auth calls silently.
    ProviderScope(retry: (_, _) => null, child: const App()),
  );
}
