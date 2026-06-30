# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project context

`tccelta_mobile` (display name "OBD2 Cockpit") is the Flutter app half of **tccelta**, a USP Poli computer-engineering undergraduate thesis (TCC). The system is an ESP32-based OBD-II dongle that reads live vehicle data over the CAN bus; this app is the companion that talks to the dongle.

The broader monorepo lives one level up at `../` and contains:
- `../esp32-firmware/` — the dongle firmware (Arduino + FreeRTOS, PlatformIO). Layered: CAN → OBD-II → ELM327 → BLE.
- `../docs/` — protocol and architecture docs. `can-e-obd2.md` (CAN frames, OBD-II services/PIDs, ISO-TP) and `rtos-freertos.md` (firmware concurrency model) are the relevant background for app↔dongle integration.

The codebase is Brazilian Portuguese (comments, identifiers in the automotive domain). Keep new comments/strings in pt-BR to match.

## Communication contract with the firmware

The app communicates with the dongle over **BLE using the Nordic UART Service**. This is the contract the app must implement (defined in `../esp32-firmware/src/connectivity/ble/BleConnectivity.h`):

- Device advertises as `OBD2Dongle`.
- Service UUID `6E400001-B5A3-F393-E0A9-E50E24DCCA9E`.
- **RX** `6E400002-…` (WRITE) — app writes **ELM327 text commands** here, e.g. `010C\r` (read PID 0x0C = engine RPM).
- **TX** `6E400003-…` (NOTIFY) — dongle pushes **ELM327 text responses**, e.g. `41 0C 17 70\r>`.

Payloads are ASCII text over BLE — not JSON/protobuf. The firmware command queue has finite depth and **drops commands under back-pressure**, so the app must handle timeouts/missing responses gracefully. Encryption/auth (`SecureBleConnectivity`) is stubbed in firmware today. For hardware-free testing, the firmware can run its `env:mock` PlatformIO profile.

> Note: no BLE package is in `pubspec.yaml` yet — the connectivity layer is unimplemented. The app today is the design system plus a showcase screen.

## Commands

Run from `mobile-app/`:

- `flutter pub get` — fetch dependencies
- `flutter run` — launch on a connected device/emulator (`--release` for release)
- `flutter analyze` — lint (strict; see below)
- `flutter test` — run all widget tests
- `flutter test test/widget_test.dart --plain-name "app builds the design system showcase"` — run a single test by name
- `dart run build_runner build --delete-conflicting-outputs` — regenerate code after editing freezed/json_serializable models (none exist yet, but the toolchain is wired)

## Architecture

Layered, under `lib/src/`, with `main.dart` as a thin bootstrap. `main.dart` currently mounts `ShowcaseScreen` directly; ProviderScope/go_router wiring and the data/domain/service feature layers are deferred (noted in `main.dart`).

- `src/core/theme/` — **design tokens**. Import via the barrel `theme.dart`. Dark-first Material 3 (`app_theme.dart`) assembled from `app_colors.dart`, `app_typography.dart` (Space Grotesk for UI, JetBrains Mono with tabular figures for data — loaded at runtime via `google_fonts`), `app_spacing.dart` (spacing + radii), and `app_effects.dart` (shadows, cyan "live" glows, motion durations with a `MotionX` accessibility bridge that respects `disableAnimations`).
- `src/ui/core/widgets/` — **design-system atoms** (the component library). Import via barrel `widgets.dart`. Includes `AppButton`, `AppTabBar`, `StatusBadge`, `SensorRow`, `StatCard`, `StatGraphCard`, and the signature `Gauge` (ring/arc270/arc180, CustomPainter in `gauge/gauge_painter.dart`) and `Sparkline`.
- `src/ui/core/icons/` — SVG icon system: `AppIcon` widget + `AppIconData` enum (`app_icons.dart`), backed by 16 SVGs in `assets/icons/` recolored at runtime via `flutter_svg`.
- `src/ui/showcase/view/showcase_screen.dart` — gallery of all tokens and atoms with simulated live sensor data. Reference/onboarding only — not a feature screen.

When adding visual elements, reuse the existing tokens and atoms rather than hardcoding colors, spacing, or text styles — the design system is the single source of truth for these.

## Component-first: always factor out reusable patterns

Whenever you build anything — a UI element, a layout, a piece of formatting/conversion logic, a state pattern — **first look for the pattern** and decide whether it belongs as a reusable component rather than inline code. This is a hard rule, not a nice-to-have:

- **Identify the pattern before writing.** If a visual block, behavior, or bit of logic could plausibly appear more than once, treat it as a candidate component. If you find yourself copy-pasting or writing something structurally similar to existing code, stop and extract it.
- **Check for an existing atom first.** Before creating anything new, search `src/ui/core/widgets/` (barrel `widgets.dart`) and the theme tokens. Reuse or extend what's there instead of duplicating. If an existing atom is *almost* right, generalize it (add a parameter/variant) rather than forking a near-copy.
- **Put it in the right place.** Reusable, domain-agnostic UI atoms go in `src/ui/core/widgets/` and are exported from the `widgets.dart` barrel. Design constants (colors, spacing, radii, typography, effects, motion) go in `src/core/theme/`, never hardcoded at the call site. Feature screens compose these atoms; they should not redefine them.
- **Keep components self-contained and configurable.** Drive variation through parameters/enums (as `Gauge` does with ring/arc270/arc180), expose sensible defaults, and avoid leaking feature-specific assumptions into a shared atom.
- **When in doubt, extract.** Prefer a small, well-named reusable widget/helper over a large inline block. A feature screen should read as a composition of named components, not a wall of layout code.

The goal: the codebase stays a thin layer of feature composition over a rich, single-source-of-truth library of reusable components and tokens.

## Stack and conventions

- **State**: `flutter_riverpod` + `hooks_riverpod` + `flutter_hooks` (declared; not yet used).
- **Routing**: `go_router` (declared; not yet wired).
- **Networking**: `dio` (for any future backend).
- **Models**: `freezed` + `json_serializable` are the intended pattern for serializable models.
- **Linting**: `analysis_options.yaml` includes `very_good_analysis` (strict). Generated `*.g.dart`/`*.freezed.dart` are excluded, and `invalid_annotation_target` is downgraded to allow freezed + json_serializable. New code is expected to pass `flutter analyze` clean.
- **Testing**: `flutter_test` for widget tests; `mocktail` is available for mocking.

## Platform notes

- Android namespace/applicationId: `br.com.malcong.tccelta_mobile`; Gradle is Kotlin DSL (`android/app/build.gradle.kts`), Java/Kotlin 17. Release currently signs with the debug key (placeholder — needs a real signing config before publishing).
- Only Android and iOS targets are configured.
