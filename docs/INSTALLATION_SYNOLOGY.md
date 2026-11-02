# Installer la passerelle Roborock → Loxone

La passerelle fonctionne dans **Container Manager** sur un Synology compatible
avec les images Linux `amd64` ou `arm64`. L’installation proposée utilise une
image publiée : vous n’avez rien à compiler. Deux conteneurs suffisent :

- `roborock-mqtt-loxone` relie vos robots à Loxone et sert l’interface web ;
- `roborock-mqtt-loxone-updater` installe une nouvelle version lorsque vous la
  demandez depuis l’interface.

Le mode Loxone HTTP direct n’a pas besoin de Mosquitto. Si votre installation
utilise déjà MQTT, conservez votre courtier et votre configuration existants.

## Vous avez déjà une installation qui fonctionne ?

**Utilisez d’abord votre mise à jour habituelle dans l’interface, page Système &
mises à jour.** Une mise à jour de l’image conserve les données montées, les
identifiants stables des robots, les noms d’entrées virtuelles et les adresses
des commandes Loxone. Aucun nouvel import Loxone n’est nécessaire pour un robot
déjà configuré.

Il n’est pas nécessaire de remplacer votre projet Compose pour bénéficier de
la nouvelle interface. Ne créez pas une seconde passerelle avec un volume neuf.
Le fichier ci-dessous est destiné aux nouvelles installations ; une migration
vers ce fichier demande de reprendre **exactement** le stockage, le port et le
réseau/courtier MQTT actuellement utilisés.

Avant une migration de projet, relevez dans Container Manager → conteneur
`roborock-mqtt-loxone` → Paramètres :

1. Le port local qui correspond au port `8080` du conteneur. Gardez ce port et
   l’adresse IP du NAS : ils sont déjà utilisés par Loxone.
2. Le dossier ou le volume monté dans `/var/lib/roborock-mqtt-loxone`. Il contient
   notamment `config.json`, la session Roborock, `device-slugs.json`, les noms de
   pièces et les paramètres de mise à jour. Sauvegardez **tout ce stockage**.
3. Si vous utilisez MQTT, le courtier et le réseau auquel il appartient. Le
   fichier simplifié ne démarre pas le courtier du profil `mqtt` existant.

Pour un dossier existant, définissez `ROBOROCK_DATA_DIR` avec son chemin absolu
dans le fichier `.env` du projet. Pour un volume Docker existant, utilisez le
nom réel du volume (souvent `roborock-mqtt-loxone_roborock-data`) et déclarez-le
externe dans le fichier Compose importé :

```yaml
volumes:
  roborock-data:
    external: true
    name: NOM_REEL_DU_VOLUME_EXISTANT
```

Dans ce cas, laissez `ROBOROCK_DATA_DIR` non défini afin que les deux services
utilisent `roborock-data`. Arrêtez l’ancien projet avant de démarrer le nouveau,
sans supprimer ses volumes. Ne lancez jamais les deux passerelles en même temps
sur les mêmes robots et entrées Loxone.

## Nouvelle installation, depuis Container Manager

1. Téléchargez [`compose.synology.yaml`](../compose.synology.yaml) depuis le
   dépôt de la version que vous souhaitez installer. La valeur par défaut
   utilise la dernière version **stable publiée**. Une modification encore
   présente uniquement dans les sources n’apparaît pas dans cette image.
2. Dans File Station, créez un dossier de projet, par exemple
   `/volume1/docker/roborock-mqtt-loxone`, et placez-y ce fichier. Le stockage des
   données sera créé automatiquement comme volume Docker.
3. Dans **Container Manager → Projet → Créer**, choisissez ce dossier et
   importez le fichier Compose. Lancez le projet. Le premier démarrage peut
   prendre quelques minutes pendant le téléchargement de l’image.
4. Ouvrez `http://ADRESSE_DU_NAS:8080`, connectez votre compte Roborock et
   configurez votre Miniserver dans la page Loxone. Les robots associés au compte
   sont récupérés automatiquement ; chaque robot possède son propre identifiant.
5. Dans **Système & mises à jour**, vérifiez que le service de mise à jour
   indique **Prêt**.

Le service de mise à jour attend que la passerelle soit démarrée pour lire son
jeton partagé. Aucun port supplémentaire n’est publié sur le NAS.

### Si le service de mise à jour ne démarre pas

Consultez son journal dans Container Manager. En cas de refus d’accès au socket
Docker, son groupe doit correspondre au groupe de `/var/run/docker.sock`.
Depuis le terminal SSH du Synology :

```sh
stat -c '%g' /var/run/docker.sock
```

Ajoutez la valeur obtenue dans un fichier `.env` placé à côté du fichier Compose,
puis reconstruisez le projet dans Container Manager :

```dotenv
DOCKER_GID=VALEUR_OBTENUE
```

### Personnaliser le port ou le dossier de données

Ces variables sont facultatives dans le fichier `.env` du projet :

```dotenv
ROBOROCK_WEB_PORT=8080
TZ=Europe/Paris
# Uniquement si vous utilisez un dossier existant plutôt que le volume Docker :
# ROBOROCK_DATA_DIR=/volume1/docker/roborock-mqtt-loxone/data
```

Un dossier monté doit être accessible en écriture par l’utilisateur `65532` du
conteneur. Pour **un nouveau dossier de données vide**, la préparation est :

```sh
sudo mkdir -p /volume1/docker/roborock-mqtt-loxone/data
sudo chown 65532:65532 /volume1/docker/roborock-mqtt-loxone/data
```

N’appliquez pas un changement récursif de propriétaire à un dossier partagé du
NAS. Le volume Docker proposé par défaut ne demande pas cette préparation.

## Mettre à jour quand vous le souhaitez

Dans **Système & mises à jour** :

1. Choisissez **Rechercher une mise à jour**.
2. Consultez les nouveautés, puis choisissez **Installer** et confirmez.
3. Laissez l’installation se terminer. La page retrouve automatiquement sa
   progression après le redémarrage de la passerelle.

L’updater sauvegarde les données, télécharge l’image autorisée, remplace
uniquement la passerelle, vérifie sa santé et sa version, puis restaure le
conteneur précédent si les contrôles échouent. L’interface peut être brièvement
indisponible pendant cette opération.

Par défaut, le mode **Prévenir seulement** peut rechercher une nouvelle version
mais **ne l’installe jamais automatiquement**. Les réglages avancés permettent
de modifier ce comportement. Une installation existante conserve sa politique ;
ouvrez les réglages si vous souhaitez revenir aux installations manuelles.

Le canal Stable suit les versions publiées. Le canal Développement (Edge), dans
les réglages avancés, suit les builds de `main` et demande une autorisation
supplémentaire pour toute installation automatique.

### Installation en ligne de commande, facultative

Dans le dossier contenant le fichier Compose :

```sh
docker compose -f compose.synology.yaml up -d
```

Pour remplacer manuellement les images des deux services depuis le registre :

```sh
docker compose -f compose.synology.yaml pull
docker compose -f compose.synology.yaml up -d
```

Cette méthode passe directement par Docker ; elle ne déclenche pas la sauvegarde
ni le retour arrière automatique de l’updater. Pour une mise à jour ordinaire,
privilégiez le bouton de l’interface. Ne supprimez pas les volumes du projet.

## Ajouter un autre robot

Ajoutez le robot au même compte dans l’application Roborock, puis redémarrez le
conteneur `roborock-mqtt-loxone` depuis Container Manager. La passerelle retrouve
la liste du compte à son démarrage. Dans Loxone, configurez uniquement les
entrées et commandes du nouveau robot. Les adresses du robot existant restent
inchangées.

## En cas de problème

- **Entrées Loxone refusées** : consultez Loxone, puis les diagnostics avancés.
  Les noms sont sensibles aux majuscules ; enregistrez vos entrées dans le
  Miniserver avant de renvoyer les états.
- **Passerelle indisponible** : consultez le journal du conteneur principal.
- **Mise à jour échouée** : consultez le résultat dans Système & mises à jour et
  le journal du conteneur updater.
- **Sauvegarde avant intervention** : conservez tout le volume de données,
  notamment les fichiers d’identifiants et de session. Une sauvegarde contient
  des secrets ; ne la publiez pas.

## Accès de l’updater

Le conteneur principal n’a aucun accès au socket Docker. L’updater est isolé,
authentifié par un jeton de fichier et limité à l’image officielle et au nom de
conteneur existant. Son accès au socket Docker lui donne néanmoins des droits
élevés sur le NAS. Ne publiez pas son port interne `8090` sur le réseau.
