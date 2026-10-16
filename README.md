# AXO Clash CLI

Teste ton bot et connecte-le au laboratoire [AXO Clash](https://clash.axopen.cloud/) depuis ton terminal.

- **`check-bot`** vérifie les réponses HTTP de ton bot avec 41 contrôles de compatibilité.
- **`relay`** relie tes bots locaux au labo pour lancer des combats depuis le site.

[Site AXO Clash](https://clash.axopen.cloud/) · [Labo](https://clash.axopen.cloud/lab) · [Téléchargements](https://github.com/axo-clash/cli/releases)

## Installation

Le CLI est disponible pour **Linux, macOS et Windows**, sur **x64 et ARM64**. Tu n'as pas besoin d'installer Go.

### Linux et macOS

```sh
curl -fsSL https://raw.githubusercontent.com/axo-clash/cli/main/install.sh | bash
```

Le binaire est installé dans `~/.local/bin`, sans `sudo`. Si ce dossier n'est pas dans ton `PATH`, le script affiche la ligne à ajouter à `~/.zshrc` ou `~/.bashrc`.

### Windows

Dans PowerShell 5.1 ou supérieur :

```powershell
irm https://raw.githubusercontent.com/axo-clash/cli/main/install.ps1 | iex
```

Le binaire est installé dans `%LOCALAPPDATA%\Programs\axo-clash`, sans droits administrateur. Le script ajoute ce dossier au `PATH` utilisateur et à la session courante.

### Vérifier l'installation et mettre à jour

```sh
axo-clash --version
```

Relance la commande d'installation pour passer à la dernière version stable. Les scripts détectent ton système et ton architecture, puis vérifient la somme **SHA-256** avant de remplacer le binaire. Linux et macOS nécessitent `curl` et `sha256sum` ou `shasum`.

Tu peux télécharger les scripts pour les lire avant exécution, ou récupérer directement le binaire adapté à ton système dans les [releases](https://github.com/axo-clash/cli/releases). Pour une installation manuelle, renomme-le en `axo-clash` (`axo-clash.exe` sur Windows), place-le dans un dossier du `PATH` et rends-le exécutable sur Linux/macOS avec `chmod +x`.

<details>
<summary>Choisir une version ou un dossier d'installation</summary>

Linux / macOS :

```sh
curl -fsSL https://raw.githubusercontent.com/axo-clash/cli/main/install.sh | AXO_CLASH_VERSION=v0.1.1 INSTALL_DIR="$HOME/.local/bin" bash
```

Windows :

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/axo-clash/cli/main/install.ps1 -OutFile install.ps1
./install.ps1 -Version v0.1.1 -InstallDir "$HOME\bin"
```

Une préversion peut être installée explicitement en précisant son tag. Elle n'est pas proposée par l'installation automatique de la dernière version stable.

</details>

## Démarrage rapide

1. Démarre ton bot en local. Il doit exposer `GET /info` et `POST /play` sous son URL de base.
2. Vérifie sa compatibilité :

   ```sh
   axo-clash check-bot --url http://127.0.0.1:9000/api/v1/bots
   ```

3. Ouvre le [labo](https://clash.axopen.cloud/lab), connecte-toi et crée une session de relais. Si l'accès direct affiche une erreur, ouvre [l'accueil du site](https://clash.axopen.cloud/) puis le menu **Labo**.
4. Connecte ton bot :

   ```sh
   axo-clash relay --bot alpha=http://127.0.0.1:9000/api/v1/bots
   ```

5. Saisis le code de la session dans l'invite du terminal, puis lance tes combats depuis le labo. Garde le CLI et ton bot démarrés pendant les combats.

## Commandes

### `relay` — connecter des bots au labo

Un seul bot :

```sh
axo-clash relay --bot alpha=http://127.0.0.1:9000/api/v1/bots
```

Plusieurs bots, par exemple deux versions de ton programme :

```sh
axo-clash relay --bot alpha=http://127.0.0.1:9000/api/v1/bots --bot beta=http://127.0.0.1:9001/api/v1/bots
```

Chaque option `--bot` associe un alias à l'URL de base du bot. Seules les adresses locales `localhost`, `127.0.0.1` et `::1` sont acceptées. Le CLI ouvre une connexion WebSocket sortante vers AXO Clash : tu n'as pas besoin d'exposer un port de ta machine sur Internet.

Le code de session est saisi dans une invite masquée. Il reste en mémoire pendant la connexion ; ne le passe pas en argument. `Ctrl+C` arrête le relais. Le CLI tente de se reconnecter en cas de coupure réseau. Si la session a expiré ou a été révoquée, crée une nouvelle session dans le labo et relance la commande.

| Option | Description |
| --- | --- |
| `--bot alias=URL` | Bot local à connecter ; option répétable. |
| `--api URL` | Autre instance de l'API ; défaut : `https://clash-api.axopen.cloud/api/v1`. Également configurable avec `AXO_CLASH_API_URL`. |
| `--dev` | Autoriser une API HTTP locale pour le développement. |
| `--help` | Afficher l'aide de la commande. |

Pour utiliser une instance locale d'AXO Clash :

```sh
axo-clash relay --api http://localhost:8080/api/v1 --dev --bot alpha=http://127.0.0.1:9000/api/v1/bots
```

### `check-bot` — vérifier la compatibilité d'un bot

Le bot doit déjà être démarré. Cette commande s'utilise sans compte AXO Clash ni connexion au labo : elle appelle directement ton bot.

```sh
axo-clash check-bot --url http://127.0.0.1:9000/api/v1/bots
```

Pour détecter des erreurs intermittentes :

```sh
axo-clash check-bot --url http://127.0.0.1:9000/api/v1/bots --repeat 5
```

| Option | Défaut | Description |
| --- | --- | --- |
| `--url` | `http://localhost:9000/api/v1/bots` | URL de base ; le CLI ajoute `/info` et `/play`. |
| `--repeat` | `1` | Nombre de passages de la suite complète. |
| `--info-timeout` | `30s` | Délai maximal pour la réponse à `/info`. |
| `--timeout` | `2s` | Délai maximal pour chaque réponse à `/play`. |
| `--help` | | Afficher l'aide de la commande. |

Si ton bot expose `/info` et `/play` directement à la racine, utilise par exemple `--url http://localhost:9000`.

Chaque passage effectue **41 contrôles** : une requête `GET /info` et 40 scénarios `POST /play`. Ils couvrent notamment les deux rôles du bot, les coordonnées du plateau, les projectiles, les champs optionnels et l'historique des actions. Chaque réponse doit être un JSON valide avec un statut HTTP 200 ; les redirections sont refusées.

Le CLI affiche les erreurs par scénario et termine avec le code :

| Code | Résultat |
| --- | --- |
| `0` | Tous les contrôles ont réussi. |
| `1` | Au moins un contrôle a échoué. |
| `2` | Arguments invalides. |

Ces contrôles vérifient la compatibilité des réponses pour les scénarios envoyés. Ils n'évaluent pas la stratégie de ton bot et ne garantissent pas sa réponse à toutes les situations de jeu.

### `--version` — afficher la version installée

```sh
axo-clash --version
```

## Développement

Go **1.23 ou supérieur** est nécessaire pour compiler depuis les sources.

```sh
git clone https://github.com/axo-clash/cli.git
cd cli
go build -o axo-clash .
go test -race -timeout 120s ./...
go vet ./...
```

Sous Windows, utilise `go build -o axo-clash.exe .`. Pour lancer une commande sans compiler manuellement, remplace `axo-clash` par `go run .`.

Les tests Go utilisent des serveurs HTTP locaux. Les tests d'installation utilisent des téléchargements simulés :

```sh
python3 scripts/test-install.py
```

Sur Windows :

```powershell
./scripts/test-install.ps1
```

## Publication d'une version

Après intégration des changements dans `main` :

```sh
git switch main
git pull --ff-only
git tag -a v0.1.1 -m "CLI v0.1.1"
git push origin v0.1.1
```

Le workflow GitHub Actions exécute les tests Go et les tests des installateurs sur les runners Linux, macOS et Windows. Il compile les six binaires sans CGO, intègre le numéro de version et publie une release avec les binaires, les scripts et `checksums.txt` une fois tous les jobs réussis. Chaque système est compilé sur son runner ; l'autre architecture est compilée de manière croisée, sans exécution sur cette architecture.

Les tags suivent `vMAJOR.MINOR.PATCH`. Un suffixe, par exemple `v0.2.0-rc.1`, produit une préversion. Aucun secret supplémentaire n'est nécessaire : seul le job de publication reçoit la permission `contents: write` du `GITHUB_TOKEN`.

Pour reprendre une publication ayant échoué avant la création de la release, relance les jobs en échec ou lance manuellement le workflow sur le tag concerné. Une release existante n'est pas écrasée ; les versions suivantes utilisent un nouveau tag.
