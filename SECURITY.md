# Security and privacy

ApplyKit does not upload documents, call an Internet API, run analytics, or automatically update. Its desktop UI communicates with the processing engine through a loopback-only (`127.0.0.1`) HTTP session using a random port and a fresh random bearer token for each launch.

ApplyKit runs with the current user's permissions and is not a hardened sandbox for hostile PDFs or images. Use a maintained Windows installation and do not disable endpoint protection or an organization's execution policies to run it.

Input count, byte and pixel caps reduce accidental resource exhaustion but are not a substitute for patched system parsers. Normal closure removes preview images and session working copies. A crash, forced termination, power loss or locked file can leave temporary files under the user's local ApplyKit data directory.

Processing reports can contain absolute local file paths. Redact those before posting an issue. PDF passwords are passed to the WinRT render child process via its environment rather than command-line arguments and are not written to reports or preferences; they still exist briefly in process memory/environment and are not protected against a privileged local observer.

Never attach private resumes, ID documents, transcripts or application materials to a public bug report. Before public distribution, repository owners should enable GitHub private vulnerability reporting or provide another private security contact.
