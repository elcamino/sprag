# Sprag Demo Asset

The launch README expects this asset path:

- `assets/demo/sprag-intake-demo.gif`

Optional higher-quality companion file:

- `assets/demo/sprag-intake-demo.mp4`

## Storyboard

Record a short 20-30 second demo with no real secrets or client data:

1. admin creates an intake page titled `Client closing documents`
2. admin copies the unguessable upload URL
3. uploader opens the page in a private window and uploads two sample files
4. uploader sees only the receipt/status screen, with no folder, listing, account, or download path
5. admin dashboard shows the submission envelope and file status
6. admin opens the evidence or manifest view if it is quick enough to show cleanly

## Capture Rules

- Use a local demo deployment and fake data only.
- Hide hostnames, admin credentials, upload slugs, receipt URLs, S3 bucket names, and private-key material.
- Keep the crop focused on the app, not the browser chrome.
- Export at 1280 px wide or lower so the GIF stays reasonable for GitHub.
- Keep the GIF under 10 MB if possible.

## Placeholder Behavior

Until the GIF is recorded, the README may point at `assets/demo/sprag-intake-demo.gif` and the readiness checker will pass only after the file exists if the checker is tightened to require the binary. The initial checker intentionally requires this README storyboard but checks the README link text rather than the binary file so documentation can land before recording.
