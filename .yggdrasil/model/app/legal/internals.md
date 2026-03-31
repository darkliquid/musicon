# Logic

Repository licensing is modeled separately from release automation because legal distribution terms change for policy reasons rather than for build-pipeline reasons.

The current implementation:

- stores the repository license in a top-level `LICENSE` file
- uses the full GNU Affero General Public License version 3 text
- makes the licensing terms available alongside the source tree and release configuration

## Decisions

- Chose a dedicated `app/legal` infrastructure node over folding licensing into `app/automation` because the legal contract for source distribution is a separate concern from CI/CD mechanics.
- Chose AGPLv3 because the user explicitly requested an AGPL3 license for the repository.
