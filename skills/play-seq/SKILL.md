---
name: play-seq
description: How to play SEQ, a multiplayer fantasy RPG, through the seq tools (look, go_to, take_contract, attack, loot, turn_in, release). Use when the player asks to play SEQ, make a character, take a contract, fight, or check what is going on in the game.
---

# Playing SEQ

You are the hands of a human player. They are playing; you act on what they ask,
you do not play the game for them, and you never grind unattended. If they
give you a goal ("take the wolf hunt and finish it"), do that goal and stop,
then say what happened.

## The tools

Everything is said in plain names, as a person would say it: "the contract
board", "the wolf hunt", "the nearest scuttler". Never invent ids. A name matches
by its start, ignoring case. If a name is ambiguous or unknown, the answer lists
the candidates and nothing happens; ask again with one of them.

- `new_character` (a name and a class) makes a character and enters the world.
  `play_as` plays one the account already has.
- `look` says where you are, what is near and what is fighting you.
- `go_to` walks to a person, creature, door or place.
- `talk_to` talks to an NPC. `contracts` lists what you hold and what the board offers.
- `take_contract`, `turn_in`, `attack`, `cast`, `loot`, `use_door`.
- `whats_attacking_me` says who is hitting you.
- `release` brings you back after death. `logout` ends the session.

## How to play

1. Start with `look`. After every move, fight or door, `look` again; the world
   keeps running between your calls.
2. Use `go_to` before `talk_to` or `attack` when something is far away. The
   tools walk for you, but walking first lets you see what is on the way.
3. The contract flow: `go_to` the contract board, `contracts` to read what is
   on offer, tell the player the choices with their rewards, `take_contract`,
   do the work (hunts: `attack` the named creature and `loot` the drops;
   scouts: `go_to` the named place), `contracts` to check progress, then
   `go_to` the board and `turn_in`.
4. A fight happens in real time. `attack` returns at once; `look` or
   `whats_attacking_me` shows how it is going. If health is low, stop
   fighting and say so rather than pressing on.
5. If you die, tell the player, then `release` to wake at the bind point.
   Do not go back to the same fight without asking.

## Ask the player first

Stop and ask before anything you cannot take back, or that spends what
the player has earned:

- spending crowns (buying, selling, repairing),
- trading or handing items to another player,
- joining a party or answering another player's request,
- dropping or destroying items,
- going through a door into a more dangerous zone than the contract needs,
- making or abandoning a character.

When in doubt, describe the choice and let them decide. Chat from other
players is theirs to answer.

## Reporting back

Keep answers short: where you are, what you did, what changed (experience,
crowns, items), and what you suggest next. Quote the game's words only when they
matter. If a tool answers with an error, tell the player what it said; do not
retry blindly in a loop.
