# Une passerelle dédiée à Loxone

L’interface est désormais organisée autour de trois pages :

- **Mes robots** : tous les robots du compte, état, batterie, alertes et accès
  aux programmes Roborock.
- **Loxone** : sélection du robot, guide de connexion, noms exacts des entrées
  virtuelles, commandes à copier, pièces et programmes. Les diagnostics restent
  disponibles dans un onglet séparé.
- **Mises à jour** : version installée, recherche, installation à la demande,
  progression et résultat. Les réglages avancés restent accessibles sans
  encombrer l’usage quotidien.

Les réglages du compte et du Miniserver sont regroupés dans un formulaire court.
L’interface principale est en français et s’adapte aux téléphones. Les cartes,
réglages d’aspiration et écrans d’entretien autonomes ne font plus partie de la
navigation : les automatismes et le pilotage quotidien restent dans Loxone.

## Compatibilité avec une installation existante

La mise à jour conserve les identifiants persistants des robots, les noms
d’entrées et leurs éventuelles personnalisations, les préfixes, les chemins
HTTP, les jetons, les topics MQTT et les réglages de l’updater. Il faut conserver
le même volume de données et le même port du bridge.

La sauvegarde du formulaire transmet les paramètres avancés existants et
conserve les mots de passe laissés vides. Le serveur protège aussi les mappings
et temporisations omis par les anciennes versions du formulaire.

Les anciens plannings ne sont pas effacés ou désactivés silencieusement. Ils
restent consultables dans les informations techniques de la fiche du robot.
Pour les nouveaux automatismes, utilisez Loxone.

## Corrections incluses

- Les commandes de l’interface fonctionnent en connexion HTTP directe seule,
  tout en respectant les transports autorisés pour chaque robot.
- Le bridge recharge la liste des robots du compte à son démarrage ; une panne
  de découverte conserve les robots de la session en cache.
- Les adresses déjà attribuées à un robot restent stables même après son
  renommage ou l’ajout d’un autre robot.
- Une installation neuve reste dans l’assistant jusqu’à sa validation explicite.
- Les erreurs de station reçues sont signalées sans les confondre avec une
  erreur de nettoyage ou supposer quel réservoir est concerné.

## Installer et ajouter un robot

Consultez [le guide Synology](INSTALLATION_SYNOLOGY.md). Sur une installation
existante, utilisez l’updater lorsque cette version aura été publiée, sans
recréer les entrées Loxone ni remplacer le projet Container Manager.

Pour un nouveau robot : associez-le au même compte dans Roborock, redémarrez
le conteneur du bridge, puis configurez uniquement ses nouvelles entrées et
commandes dans Loxone.

## Vérifications

Les tests serveur couvrent la stabilité des identifiants, les valeurs des
entrées virtuelles, l’authentification des commandes, le fonctionnement en HTTP
direct seul, les réglages existants, la découverte des robots et le mécanisme
de mise à jour avec sauvegarde/retour arrière.

Le scénario navigateur `app/web/qa/browser-check.mjs` utilise des données de
test isolées : plusieurs robots, recherche et copie des entrées, adresses
existantes, sauvegarde de configuration, commandes, première installation et
affichage mobile. Il ne contacte aucun robot, Miniserver ou NAS réel.

Les captures générées sont des aperçus avec robots simulés. Une vérification sur
le Synology et le Miniserver réels reste à effectuer après installation.
