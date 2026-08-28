# WotLK starter catalog

The built-in catalog contains 51 specialization-aware level-80 packages: 16 Furious Gladiator (Season 6), 16 Relentless Gladiator (Season 7), and 19 Conqueror tier-8 packages. Every playable Wrath class is covered; hybrid classes have separate damage, healing, and tank packages where the original game has distinct sets.

The names, set relationships, and consumable IDs were verified against the upstream [AzerothCore WotLK `item_template`](https://github.com/azerothcore/azerothcore-wotlk/blob/master/data/sql/base/db_world/item_template.sql) dataset (client build 12340 data).

## Item source of truth

At startup, the portal resolves each armor set by its canonical English WotLK name and `itemset` relationship in AzerothCore's installed `item_template` table. It accepts only level-80-era records with a real client `VerifiedBuild`, then snapshots the resulting item IDs into `portal_product_items`. This avoids silently sending a Cataclysm or later item with a reused marketing label and excludes AzerothCore's internal test duplicates.

Each resolved package contains:

- The complete five-piece S6, S7, or T8 armor set for its specialization.
- Ten epic red or blue role gems plus one appropriate meta gem.
- Five role-appropriate armor enhancements: head arcanum, shoulder inscription, leg armor or spellthread, chest enchant, and glove enchant.
- A level-80 service.

The role kits use the canonical Wrath items: Bold, Delicate, or Runed Cardinal Ruby; Solid Majestic Zircon; Relentless, Chaotic, Insightful, or Austere Earthsiege Diamond; and matching level-80 enchant scrolls.

## Delivery behavior

Gems and enchantments are intentionally delivered as items. AzerothCore's standard `send items` command cannot safely create an already-enchanted, pre-socketed item instance. The player equips the armor and applies the supplied gems and scrolls in game.

Bundles larger than one mailbox message are split into groups of twelve attachments. The entire order remains in `review` after an ambiguous SOAP failure so staff can inspect already-sent mail before retrying.

If any required set piece or supply cannot be resolved in the installed world database, that package is not created. Startup logs report the number of complete packages loaded. This fail-closed behavior prevents mislabeled or partially populated products.
