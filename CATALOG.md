# WotLK starter catalog

The built-in catalog contains 51 specialization-aware level-80 packages: 16 Furious Gladiator (Season 6), 16 Relentless Gladiator (Season 7), and 19 Conqueror tier-8 packages. Every playable Wrath class is covered; hybrid classes have separate damage, healing, and tank packages where the original game has distinct sets.

The names, set relationships, and consumable IDs were verified against the upstream [AzerothCore WotLK `item_template`](https://github.com/azerothcore/azerothcore-wotlk/blob/master/data/sql/base/db_world/item_template.sql) dataset (client build 12340 data).

## Item source of truth

At startup, the portal resolves each armor set by its canonical English WotLK name and `itemset` relationship in AzerothCore's installed `item_template` table. It accepts only level-80-era records with a real client `VerifiedBuild`, then snapshots the resulting item IDs into `portal_product_items`. This avoids silently sending a Cataclysm or later item with a reused marketing label and excludes AzerothCore's internal test duplicates.

Each resolved package is now a complete specialization loadout containing:

- The complete five-piece S6, S7, or T8 armor set for its specialization.
- A matching neck, cloak, wrist, waist, and feet item.
- Two distinct rings and two distinct trinkets.
- A class-appropriate weapon configuration: two-hander, dual wield, main-hand/off-hand, or one-hand/shield as appropriate.
- A ranged weapon, wand, thrown weapon, or class relic when the class has that slot.
- Twenty role gems plus one appropriate meta gem. S6 and T8 receive Ulduar-era rare gems; S7 receives Trial of the Crusader-era epic gems.
- A complete enchant kit covering head, shoulders, legs, chest, gloves, weapon, cloak, wrists, and boots.
- A level-80 service.

S6 and S7 use their matching Gladiator off-pieces, jewelry, and weapons. Every PvP package includes a crowd-control-removing Medallion; the order snapshot selects `Medallion of the Alliance` or `Medallion of the Horde` from the chosen character's race. The remaining S6/S7 gear is faction-neutral. T8 uses curated Ulduar items at item level 219–239, with armor type, role, dual-wield rules, shields, and relics selected per specialization. Death knight tanks correctly receive a two-handed weapon rather than a shield.

## Delivery behavior

Gems and enchantments are intentionally delivered as items. AzerothCore's standard `send items` command cannot safely create an already-enchanted, pre-socketed item instance. The player equips the armor and applies the supplied gems and scrolls in game. Physical dual-wield packages receive two weapon enchant scrolls.

Bundles larger than one mailbox message are split into groups of twelve attachments. The entire order remains in `review` after an ambiguous SOAP failure so staff can inspect already-sent mail before retrying.

If any required set piece or supply cannot be resolved in the installed world database, that package is not created. Startup logs report the number of complete packages loaded. This fail-closed behavior prevents mislabeled or partially populated products.
