# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.1] - 2025-05-26

### Fixed

- Boards with lots of images jump around a ton when loading, and if you refresh
  quickly a bunch of times, sometimes cards would end up in different places!
  this determines the placement when it first loads, which makes loading the
  page a lot less jarring

## [0.3.0] - 2025-05-25

### Fixed

- Boards with lots of images that aren't resized are pretty slow loading,
  this adds some fixes for that, not necessarily a bug fix but a new feature
  that fixes an annoyance

## [0.2.1] - 2025-05-25

### Fixed

- Notchiness when scrolling on mobile, would also skip to the top when scrolling
  down and then back up again

## [0.2.0] - 2025-05-23

### Added

- Logo ✨

## [0.1.1] - 2025-05-23

### Fixed

- Removed a useless paragraph in page output
- Fix a bug when scrolling the modal
  (once you got to the end it scrolled the page behind)

## [0.1.0] - 2025-05-22

### Added

- Initial release
