// Self-hosted (global) Renovate config used by .github/workflows/renovate.yaml.
// Options here cannot be overridden by the repository config (.github/renovate.json).
module.exports = {
  platform: 'github',
  repositories: ['chez-shanpu/tastecheck'],
  onboarding: false,
  requireConfig: 'required',
  // Commit through the GitHub API so that commits are signed by the App;
  // main requires signed commits.
  platformCommit: 'enabled',
  // The only command postUpgradeTasks may run.
  allowedCommands: ['^/tmp/aqua-root/bin/aqua update-checksum --prune$'],
  secrets: {
    AQUA_GITHUB_TOKEN: process.env.AQUA_GITHUB_TOKEN,
  },
  customEnvVariables: {
    AQUA_ROOT_DIR: process.env.AQUA_ROOT_DIR,
    AQUA_GITHUB_TOKEN: '{{ secrets.AQUA_GITHUB_TOKEN }}',
  },
};
