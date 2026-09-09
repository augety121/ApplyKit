# Security and privacy

ApplyKit does not upload documents, call a network API, run analytics, or automatically update. It runs with the current user's permissions and is not a hardened sandbox for hostile PDFs.

Use a maintained Windows installation and rebuild public releases with a supported Go toolchain. Input caps limit memory and disk use but are not a substitute for patched system parsers. Do not disable endpoint protection or bypass an organization's execution policies to run the application.

Normal closure removes preview images and session files. A crash, forced termination, power loss or locked file can leave temporary files. See the user guide for cache locations. Reports can include absolute paths; redact those before posting issues. PDF passwords are passed via child-process environment only, not command-line arguments or report JSON. They still exist briefly in process memory/environment and are not protected against a privileged local observer.

Do not publish private materials in bug reports. A maintainer should enable GitHub private vulnerability reporting before public distribution. No private reporting endpoint is claimed to exist for a repository that has not yet been created.
