import 'package:douyin_capture/app/app_state.dart';
import 'package:douyin_capture/features/capture/presentation/capture_page.dart';
import 'package:douyin_capture/features/jobs/presentation/history_page.dart';
import 'package:douyin_capture/features/jobs/presentation/job_detail_page.dart';
import 'package:douyin_capture/features/login/presentation/login_page.dart';
import 'package:douyin_capture/features/settings/presentation/settings_page.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

GoRouter buildRouter(AppState state) => GoRouter(
  initialLocation: '/capture',
  refreshListenable: state,
  redirect: (context, routeState) {
    if (state.initialising) return null;
    if (!state.authenticated) return '/login';
    if (routeState.matchedLocation == '/login') return '/capture';
    final notificationJob = state.takeNotificationJob();
    if (notificationJob != null) return '/jobs/$notificationJob';
    return null;
  },
  routes: <RouteBase>[
    GoRoute(path: '/login', builder: (context, routeState) => LoginPage(state: state)),
    StatefulShellRoute.indexedStack(
      builder: (context, routeState, shell) => _AppShell(shell: shell),
      branches: <StatefulShellBranch>[
        StatefulShellBranch(routes: <RouteBase>[GoRoute(path: '/capture', builder: (context, routeState) => CapturePage(state: state))]),
        StatefulShellBranch(routes: <RouteBase>[GoRoute(path: '/history', builder: (context, routeState) => HistoryPage(state: state))]),
        StatefulShellBranch(routes: <RouteBase>[GoRoute(path: '/settings', builder: (context, routeState) => SettingsPage(state: state))]),
      ],
    ),
    GoRoute(path: '/jobs/:id', builder: (context, routeState) => JobDetailPage(state: state, jobId: routeState.pathParameters['id']!)),
  ],
);

class _AppShell extends StatelessWidget {
  const _AppShell({required this.shell}); final StatefulNavigationShell shell;
  @override Widget build(BuildContext context) { final wide = MediaQuery.sizeOf(context).width >= 760; final destinations = const <NavigationDestination>[NavigationDestination(icon: Icon(Icons.add_circle_outline), selectedIcon: Icon(Icons.add_circle), label: '提取'), NavigationDestination(icon: Icon(Icons.history), label: '历史'), NavigationDestination(icon: Icon(Icons.settings_outlined), selectedIcon: Icon(Icons.settings), label: '设置')]; if (wide) { return Scaffold(body: Row(children: <Widget>[NavigationRail(selectedIndex: shell.currentIndex, onDestinationSelected: (index) => shell.goBranch(index, initialLocation: index == shell.currentIndex), labelType: NavigationRailLabelType.all, destinations: destinations.map((item) => NavigationRailDestination(icon: item.icon, selectedIcon: item.selectedIcon, label: Text(item.label))).toList()), const VerticalDivider(width: 1), Expanded(child: shell)])); } return Scaffold(body: shell, bottomNavigationBar: NavigationBar(selectedIndex: shell.currentIndex, onDestinationSelected: (index) => shell.goBranch(index, initialLocation: index == shell.currentIndex), destinations: destinations)); }
}
