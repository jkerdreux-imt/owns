# TODO — Branche DoH

- [x] Créer branche `doh-experiment`
- [x] `pool.go` — supprimer (plus besoin, http.Client gère le pooling)
- [x] `config.go` — virer les constantes pool, ajouter timeout DoH
- [x] `forward.go` — remplacer exchange() par dohExchange(), modifier extractServerURL
- [x] `go build` — OK
- [ ] Test manuel — lancer owns avec une conf DoH
