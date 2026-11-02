# Vérification de la refonte Loxone

Version préparée à partir de la révision `635502f`, le 1er novembre 2026.

## Contrôles effectués

- Suite complète serveur, y compris le détecteur de concurrence :
  `go test -race -count=1 ./...` — réussie.
- Analyse statique et compilation serveur : `go vet ./...` et `go build ./...`
  — réussies.
- Contrôle TypeScript, tests unitaires frontend et compilation de production :
  `tsc -b`, `vitest run` et `vite build` — réussis.
- Vérification des règles de tags Docker :
  `scripts/container-tags_test.sh` — réussie.
- Vérification navigateur sur la compilation de production : 12 scénarios,
  15 captures ordinateur/mobile, aucune erreur JavaScript ou console.

Le navigateur a notamment vérifié les noms d’entrées personnalisés, les chemins
HTTP des commandes, l’isolation entre deux robots, les commandes en HTTP direct,
les programmes Roborock, la conservation des réglages et du jeton, les alertes
station, les erreurs réseau et la première connexion.

## Périmètre

Les scénarios navigateur utilisent des réponses de test et ne contactent aucun
robot, Miniserver ou NAS. Le fichier Compose a été vérifié structurellement ;
aucun moteur Docker n’est disponible dans l’environnement de préparation.

Le Synology existant n’a pas été modifié. La nouvelle image doit être publiée
avant que cette refonte puisse être installée avec l’updater. Le premier essai
sur le matériel réel reste à faire après installation.

## Reproduire le contrôle navigateur

Depuis `app/web`, utilisez `qa/browser-check.mjs`. La documentation de ses
options se trouve dans `qa/README.md`. Avec `QA_START_SERVER=0` et `QA_ORIGIN`,
le même scénario vérifie la version produite par `vite build` servie par
`vite preview`.
