import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/router.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';

/// Shared localisation setup: Vietnamese is the default; English when the
/// device asks for it.
const localizationsDelegates = <LocalizationsDelegate<Object>>[
  AppLocalizations.delegate,
  // Deprecated on the main channel in favour of package:material_ui; kept
  // for stable-channel compatibility (the app uses package:flutter/material).
  // ignore: deprecated_member_use
  GlobalMaterialLocalizations.delegate,
  GlobalWidgetsLocalizations.delegate,
  // Same as above (package:cupertino_ui on main).
  // ignore: deprecated_member_use
  GlobalCupertinoLocalizations.delegate,
];

const supportedLocales = [Locale('vi'), Locale('en')];

Locale resolveLocale(List<Locale>? device, Iterable<Locale> supported) {
  for (final l in device ?? const <Locale>[]) {
    if (l.languageCode == 'en') return const Locale('en');
    if (l.languageCode == 'vi') return const Locale('vi');
  }
  return const Locale('vi');
}

class App extends ConsumerWidget {
  const App({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp.router(
      onGenerateTitle: (context) => AppLocalizations.of(context).appTitle,
      theme: ThemeData(colorSchemeSeed: Colors.indigo),
      darkTheme: ThemeData(
        colorSchemeSeed: Colors.indigo,
        brightness: Brightness.dark,
      ),
      localizationsDelegates: localizationsDelegates,
      supportedLocales: supportedLocales,
      localeListResolutionCallback: resolveLocale,
      routerConfig: ref.watch(routerProvider),
    );
  }
}
