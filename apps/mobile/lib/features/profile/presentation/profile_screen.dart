import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_ory_auth_mobile/app/providers.dart';
import 'package:go_ory_auth_mobile/app/routes.dart';
import 'package:go_ory_auth_mobile/core/kratos/kratos_messages.dart';
import 'package:go_ory_auth_mobile/core/network/app_failure.dart';
import 'package:go_ory_auth_mobile/features/profile/domain/me.dart';
import 'package:go_ory_auth_mobile/l10n/gen/app_localizations.dart';
import 'package:go_router/go_router.dart';

/// `GET /v1/me`.
final FutureProvider<Me> meProvider = FutureProvider.autoDispose<Me>(
  (ref) => ref.watch(profileRepositoryProvider).getMe(),
  retry: (_, _) => null,
);

class ProfileScreen extends ConsumerWidget {
  const ProfileScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final me = ref.watch(meProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.profileTitle),
        actions: [
          IconButton(
            tooltip: l10n.settingsTitle,
            icon: const Icon(Icons.security),
            onPressed: () => context.push(Routes.settings),
          ),
        ],
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: () => ref.refresh(meProvider.future),
          child: switch (me) {
            AsyncData(:final value) => _ProfileBody(me: value),
            AsyncError(:final error) => ListView(
              padding: const EdgeInsets.all(24),
              children: [
                Text(
                  error is AppFailure
                      ? failureMessage(l10n, error)
                      : l10n.genericError,
                  key: const Key('profile.error'),
                ),
                const SizedBox(height: 12),
                FilledButton(
                  onPressed: () => ref.invalidate(meProvider),
                  child: Text(l10n.retry),
                ),
              ],
            ),
            _ => const Center(child: CircularProgressIndicator()),
          },
        ),
      ),
    );
  }
}

class _ProfileBody extends ConsumerWidget {
  const _ProfileBody({required this.me});

  final Me me;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final scheme = Theme.of(context).colorScheme;
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        ListTile(
          title: Text(l10n.email),
          subtitle: Text(me.email, key: const Key('profile.email')),
          trailing: Chip(
            key: const Key('profile.verifiedBadge'),
            avatar: Icon(
              me.emailVerified ? Icons.verified : Icons.error_outline,
              size: 18,
            ),
            label: Text(me.emailVerified ? l10n.verified : l10n.notVerified),
            backgroundColor: me.emailVerified ? null : scheme.errorContainer,
          ),
        ),
        if (!me.emailVerified)
          Align(
            alignment: Alignment.centerRight,
            child: TextButton(
              onPressed: () {
                ref.read(authControllerProvider.notifier).requireVerification();
                context.go(Routes.verifyEmail);
              },
              child: Text(l10n.verifyNow),
            ),
          ),
        ListTile(
          title: Text('${l10n.firstName} / ${l10n.lastName}'),
          subtitle: Text(me.name?.full ?? '-'),
        ),
        ListTile(
          title: Text(l10n.displayName),
          subtitle: Text(
            me.displayName ?? '-',
            key: const Key('profile.displayName'),
          ),
        ),
        ListTile(title: Text(l10n.locale), subtitle: Text(me.locale)),
        ListTile(
          key: const Key('profile.personalInfo'),
          leading: const Icon(Icons.badge_outlined),
          title: Text(l10n.personalInfoTitle),
          trailing: const Icon(Icons.chevron_right),
          onTap: () => context.push(Routes.personalInfo),
        ),
        const SizedBox(height: 12),
        FilledButton.icon(
          key: const Key('profile.edit'),
          icon: const Icon(Icons.edit),
          label: Text(l10n.edit),
          onPressed: () => context.push(Routes.editProfile, extra: me),
        ),
      ],
    );
  }
}
