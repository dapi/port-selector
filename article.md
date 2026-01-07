
```bash
$
$ cd ~/projects/project-a
$ npm run dev -- --port $(port-selector)

$ port-selector
3000

$ cd ~/projects/project-b
$ port-selector
3001

$ cd ~/projects/project-a
$ port-selector
3000  # Тот же порт!

$ port-selector --list
3000 busy ~/projects/project-a
3001 busy ~/projects/project-b
3002 free
```
