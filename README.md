# CLI AXO Clash

Vérifie un bot déjà démarré ou relie tes bots locaux au labo cloud. Le relais utilise `coder/websocket` et `x/term` pour WSS et la saisie masquée.

## Installation

Aucune installation de Go nécessaire. Les releases fournissent des binaires **Linux, macOS et Windows**, en **x64 et ARM64**.

### Linux / macOS

```sh
curl -fsSL https://raw.githubusercontent.com/axo-clash/cli/HEAD/install.sh | bash
axo-clash --version
```

Le script installe la dernière release stable dans `~/.local/bin`, sans sudo, et indique la ligne à ajouter au `PATH` si nécessaire. Pour choisir la version ou le dossier :

```sh
curl -fsSL https://raw.githubusercontent.com/axo-clash/cli/HEAD/install.sh | AXO_CLASH_VERSION=v1.0.0 INSTALL_DIR="$HOME/.local/bin" bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/axo-clash/cli/HEAD/install.ps1 | iex
axo-clash --version
```

Le script installe dans `%LOCALAPPDATA%\Programs\axo-clash` et ajoute ce dossier au `PATH` utilisateur et à la session courante, sans droits administrateur. Pour choisir la version ou le dossier, télécharge le script puis exécute-le :

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/axo-clash/cli/HEAD/install.ps1 -OutFile install.ps1
./install.ps1 -Version v1.0.0 -InstallDir "$HOME\bin"
```

Les scripts vérifient le SHA-256 publié dans la release avant de remplacer le binaire. Relance la même commande pour mettre à jour. Tu peux aussi télécharger et inspecter les scripts avant de les exécuter, ou installer manuellement un binaire depuis [GitHub Releases](https://github.com/axo-clash/cli/releases). Linux nécessite `curl` et `sha256sum` ou `shasum` ; macOS fournit `shasum`. PowerShell 5.1 ou supérieur est requis sous Windows.

## Publier une version

Depuis le dépôt `cli`, après avoir poussé les changements à publier :

```sh
git tag -a v1.0.0 -m "CLI v1.0.0"
git push origin v1.0.0
```

Le workflow `.github/workflows/release.yml` :

1. Valide le tag `vMAJOR.MINOR.PATCH` (suffixe de préversion autorisé, ex. `v1.1.0-rc.1`).
2. Exécute les tests Go avec le détecteur de courses et `go vet` sur les runners Linux, macOS et Windows, ainsi que les tests des installateurs.
3. Compile les six binaires sans CGO, avec la version intégrée. Chaque OS est compilé sur son runner ; l'autre architecture est compilée de manière croisée, sans exécution sur cette architecture.
4. Publie une GitHub Release avec les binaires `axo-clash_{linux|darwin|windows}_{amd64|arm64}[.exe]`, `checksums.txt` et les deux scripts, après réussite de tous les builds.

Un tag avec suffixe produit une **préversion**, ignorée par l'installation `latest`. Pour relancer la publication après un échec avant création de la release, utilise « Re-run failed jobs » ou lance manuellement le workflow en sélectionnant le tag existant. Une release déjà publiée n'est pas écrasée : publie un nouveau tag. Le workflow utilise le `GITHUB_TOKEN` intégré avec `contents: write` uniquement pour le job de publication ; aucun secret supplémentaire requis. Les règles GitHub du dépôt doivent autoriser ce job à créer des releases.

Les commandes d'installation deviennent utilisables une fois les scripts poussés sur la branche par défaut et la première release stable publiée. Pour développer depuis les sources, Go 1.23 ou supérieur est requis ; les exemples `go run .` ci-dessous se lancent depuis ce dossier. Après installation, remplace `go run .` par `axo-clash`.

## Relais du labo

Connecte-toi au front, ouvre le labo et crée une session. Démarre tes bots puis lance :

```sh
go run . relay \
  --bot alpha=http://127.0.0.1:9000/api/v1/bots \
  --bot beta=http://127.0.0.1:9001/api/v1/bots
```

Saisis le code du labo dans l'invite masquée. Il n'est jamais passé en argument. Le CLI utilise `https://clash-api.axopen.cloud/api/v1` par défaut. Pour une autre instance, utilise `--api` ou `AXO_CLASH_API_URL` ; une URL sans chemin reçoit automatiquement le préfixe `/api/v1`. Le credential reste en mémoire jusqu'à la fin du processus. `Ctrl+C` ferme la connexion.

Les URL des bots restent sur ton poste. Seules les adresses loopback sont acceptées (`localhost`, `127.0.0.1`, `::1`) ; les doublons normalisés sont refusés. Le relais accepte seulement `/info` et `/play`, sans redirection ni transmission des credentials cloud. Les réponses et messages sont bornés à 256 Kio. Une requête par bot et quatre appels simultanés par connexion au maximum ; `/info` dispose de 30 secondes ; chaque action `/play` conserve le budget serveur de deux secondes, réseau compris.

Une déconnexion annule les appels en cours. La reconnexion utilise un délai croissant, sans rejouer les actions du combat interrompu. Une session révoquée ou expirée nécessite un nouveau code. Pour développer avec une API locale uniquement :

```sh
go run . relay --api http://localhost:8080/api/v1 --dev \
  --bot alpha=http://127.0.0.1:9000/api/v1/bots
```

## Vérification du contrat

Depuis ce dossier :

```sh
go run . check-bot -url http://localhost:9000/api/v1/bots

# Répéter toute la suite pour détecter les erreurs intermittentes
go run . check-bot -url http://localhost:9000/api/v1/bots -repeat 5

# Construire le CLI
go build -o axo-clash .
./axo-clash check-bot -url http://localhost:9000/api/v1/bots -timeout 2s
```

`-url` est l'URL de base exacte enregistrée pour le bot : le CLI ajoute `/info` et `/play`. Elle vaut `http://localhost:9000/api/v1/bots` par défaut. Un bot exposant directement `/info` et `/play` peut utiliser `-url http://localhost:9000`.

Chaque passage effectue **41 vérifications** :

- `GET /info` : HTTP **200**, réponse JSON avec `name` et `version` textuels et non vides.
- 40 requêtes `POST /play` : HTTP **200**, réponse JSON avec une `action` de l'enum backend (`MOVE_*`, `SHOOT_*`, `UNKNOWN`).
- Deux rôles (`FIRST_BOT`, `SECOND_BOT`), plateau 8 × 8 indexé `[y][x]`, cases vides, positions aux bords, champs optionnels omis et `null` explicites, listes d'items vides, quatre directions de projectiles, plusieurs projectiles sur une case et projectiles sur un bot.
- Historique avec toutes les actions, actions interdites, dates ISO 8601, temps de réponse, vie basse et `faster`.
- Variantes `contrat/…` pour toutes les valeurs d'état, statut, gagnant, raison de fin et raison d'échec. Elles testent le décodage du contrat complet ; le moteur appelle normalement `/play` pendant une partie en cours.

Le délai maximal est de **30 secondes pour `/info`** et **2 secondes pour `/play`**, conformément au moteur. `-info-timeout` et `-timeout` permettent de les ajuster séparément pour le diagnostic. Les redirections sont des échecs : elles ne doivent pas masquer un endpoint absent. La suite continue après un échec et affiche le scénario, le statut HTTP ou l'erreur de réponse. Codes de sortie : **0** succès, **1** au moins un échec, **2** arguments invalides.

Les scénarios suivent les DTO Java `GameBotDTO`, `GameDTO`, `BotDTO`, `CaseDTO`, `ItemDTO`, `ActionDTO`, les enums et `BotWebService` dans `application-backend/src/main/java/fr/axopen/axoclash`. `application.yml` configure `NON_NULL`, d'où les tests des champs omis. Les annotations de `BotController` fournissent le contrat OpenAPI des deux endpoints ; aucune spécification OpenAPI exportée n'est présente dans ce workspace.

La réussite confirme les réponses observées pour ces scénarios. Elle ne prouve pas que le bot exploite chaque champ reçu ni qu'il répondra 200 à toute entrée possible. Les JSON malformés ne sont pas envoyés : le contrat autorise leur rejet en 400. `UNKNOWN` est accepté par le contrat, sans évaluer la pertinence tactique de l'action choisie.

Tests du vérificateur avec serveurs HTTP locaux :

```sh
go test ./...
go vet ./...
```
