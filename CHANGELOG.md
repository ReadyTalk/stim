# Stim Changelog

## 0.0.4 (In development)

### BREAKING CHANGES
* Changing parameter for `stim aws login` ...

### Features
* Added STS support.  This allows `stim aws login` to provision STS credentials with the IAM credentials it received from Vault.  This increases the utility as you can now provision web console access that is limited to the user's IAM credential role.

### Improvements
* Updated the logger for more robustness and readability

### Bug Fixes
* Fixed issue with `kube config` wherein the `--namespace` argument  was not being used correctly