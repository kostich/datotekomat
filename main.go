package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kostich/datotekomat/sfat"
)

type config struct {
	verbose  bool
	testTime bool

	totalSectors   int
	bytesPerSector int
	totalFSEntries int
	label          string
}

var cfg = config{
	totalSectors:   8,
	bytesPerSector: 16,
	totalFSEntries: 4,
	label:          "ЈСДАТ-1", // JSDAT-1 (jednostavni sistem datoteka - izdanje 1)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	// parse global flags (before the subcommand)
	globalFlags := flag.NewFlagSet("датотекомат", flag.ContinueOnError)
	var help bool
	var version bool
	globalFlags.BoolVar(&help, "п", false, "приказ помоћи")
	globalFlags.BoolVar(&cfg.verbose, "в", false, "више појединости током рада")
	globalFlags.BoolVar(&version, "издање", false, "приказ издања")
	globalFlags.BoolVar(&cfg.testTime, "пв", false, "пробна времена")

	if err := globalFlags.Parse(os.Args[1:]); err != nil {
		printHelp()
		os.Exit(1)
	}

	if help {
		printHelp()
		os.Exit(0)
	}

	if version {
		fmt.Println("издање 0.1.0")
		os.Exit(0)
	}

	remaining := globalFlags.Args()
	if len(remaining) == 0 {
		printHelp()
		os.Exit(1)
	}

	subcmd := remaining[0]
	subcmdArgs := remaining[1:]

	switch subcmd {
	case "фмт": // formatiraj (format a new fs)
		runFormat(subcmdArgs)
	case "осб": // osobine (show fs details)
		runShowDetails(subcmdArgs)
	case "кпу": // kpu (kopiraj unutar), copy a file from host to fs
		runCopyIn(subcmdArgs)
	case "кпс": // kps (kopiraj spolja), copy a file from fs to host
		runCopyOut(subcmdArgs)
	case "лс": // ls (listaj), list files
		runList(subcmdArgs)
	case "пнј": // pnj (preimenuj), rename
		runRename(subcmdArgs)
	case "обш": // obš (obriši), delete
		runDelete(subcmdArgs)
	case "нпфас": // npfas (napravi fasciklu), make a folder
		runMakeFolder(subcmdArgs)
	case "прист": // prist (pristup), access (change mode)
		runChangeMode(subcmdArgs)
	case "иб": // ib (identifikacioni broj), id (uid/gid)
		runChangeID(subcmdArgs)
	case "вежи": // veži, link
		runLink(subcmdArgs)
	case "упдч": // upiši podizača (updč), write bootloader
		runWriteBootloader(subcmdArgs)
	case "етк": // etk (etiketa), rename label
		runRenameLabel(subcmdArgs)
	case "врм": // vrm (vreme), change timestamp
		runChangeTimestamp(subcmdArgs)
	case "стабло": // stablo, tree view
		runListTree(subcmdArgs)
	default:
		fmt.Printf("датотекомат: непозната наредба \"%v\"\n", subcmd)
		printHelp()
		os.Exit(1)
	}
}

// subcommand handlers

func runFormat(args []string) {
	fmtFlags := flag.NewFlagSet("фмт", flag.ExitOnError)
	fmtFlags.IntVar(&cfg.totalSectors, "ус", cfg.totalSectors, "укупно сектора")
	fmtFlags.IntVar(&cfg.bytesPerSector, "бпс", cfg.bytesPerSector, "бајтова по сектору")
	fmtFlags.IntVar(&cfg.totalFSEntries, "уст", cfg.totalFSEntries, "укупно ставки")
	fmtFlags.StringVar(&cfg.label, "етк", cfg.label, "етикета")
	fmtFlags.Parse(args)

	positional := fmtFlags.Args()
	if len(positional) < 1 {
		printError(fmt.Errorf("потребна путања за нови систем датотека"))
	}
	writeNewFilesystem(positional[0])
}

func runShowDetails(args []string) {
	if len(args) < 1 {
		printError(fmt.Errorf("потребна путања постојећег система датотека"))
	}
	showFSDetails(args[0])
}

func runCopyIn(args []string) {
	if len(args) < 3 {
		printError(fmt.Errorf("потребни: <датотека> <унутрашња путања> <систем датотека>"))
	}
	copyFileIn(args[0], args[1], args[2])
}

func runCopyOut(args []string) {
	if len(args) < 3 {
		printError(fmt.Errorf("потребни: <унутрашња путања> <спољна путања> <систем датотека>"))
	}
	copyFileOut(args[0], args[1], args[2])
}

func runList(args []string) {
	if len(args) < 2 {
		printError(fmt.Errorf("потребни: <путања> <систем датотека>"))
	}
	listEntries(args[1], args[0])
}

func runListTree(args []string) {
	if len(args) < 2 {
		printError(fmt.Errorf("потребни: <путања> <систем датотека>"))
	}
	listTree(args[1], args[0])
}

func runRename(args []string) {
	if len(args) < 3 {
		printError(fmt.Errorf("потребни: <постојећа путања> <нови назив> <систем датотека>"))
	}
	renameEntry(args[0], args[1], args[2])
}

func runDelete(args []string) {
	if len(args) < 2 {
		printError(fmt.Errorf("потребни: <путања> <систем датотека>"))
	}
	deleteEntry(args[0], args[1])
}

func runMakeFolder(args []string) {
	if len(args) < 2 {
		printError(fmt.Errorf("потребни: <путања> <систем датотека>"))
	}
	createFolder(args[0], args[1])
}

func runChangeMode(args []string) {
	if len(args) < 3 {
		printError(fmt.Errorf("потребни: <путања> <овлашћење> <систем датотека>"))
	}
	changeMode(args[0], args[1], args[2])
}

func runChangeID(args []string) {
	if len(args) < 3 {
		printError(fmt.Errorf("потребни: <путања> <ид> <систем датотека>"))
	}
	changeUIDGID(args[0], args[1], args[2])
}

func runLink(args []string) {
	if len(args) < 3 {
		printError(fmt.Errorf("потребни: <одредиште> <назив везе> <систем датотека>"))
	}
	createLink(args[0], args[1], args[2])
}

func runWriteBootloader(args []string) {
	if len(args) < 2 {
		printError(fmt.Errorf("потребни: <путања подизача> <систем датотека>"))
	}
	writeBootloader(args[0], args[1])
}

func runRenameLabel(args []string) {
	if len(args) < 2 {
		printError(fmt.Errorf("потребни: <нова етикета> <систем датотека>"))
	}
	renameLabel(args[0], args[1])
}

func runChangeTimestamp(args []string) {
	if len(args) < 4 {
		printError(fmt.Errorf("потребни: <путања> <ознака: н|и|п> <време: дд.мм.гггг-чч:мм:сс> <систем датотека>"))
	}
	changeTimestamp(args[0], args[1], args[2], args[3])
}

// help output

func printHelp() {
	fmt.Println("Употреба: датотекомат [опције] <наредба> [аргументи наредбе]")
	fmt.Println("Опције:")
	fmt.Println("  -п        приказ помоћи")
	fmt.Println("  -в        више појединости током рада")
	fmt.Println("  -издање   приказ издања програма")
	fmt.Println("  -пв       коришћење пробних времена")
	fmt.Println()
	fmt.Println("Наредбе:")
	fmt.Println("  фмт [заставице] <путања>     нови систем датотека унутар дате путање")
	fmt.Println("    заставице:")
	fmt.Println("      -ус <број>     укупно сектора (подразумевано 8)")
	fmt.Println("      -бпс <број>    бајтова по сектору (подразумевано 16)")
	fmt.Println("      -уст <број>    укупно ставки (подразумевано 4)")
	fmt.Println("      -етк <назив>   етикета (подразумевано \"ЈСДАТ-1\")")
	fmt.Println()
	fmt.Println("  осб <систем датотека>                                    	- приказ особина")
	fmt.Println("  кпу <датотека> <унутрашња путања> <систем датотека>     	- копирање унутар")
	fmt.Println("  кпс <унутрашња путања> <спољна путања> <систем датотека>	- копирање споља")
	fmt.Println("  лс <путања> <систем датотека>                           	- листање садржаја")
	fmt.Println("  стабло <путања> <систем датотека>                      	- приказ стабла")
	fmt.Println("  пнј <путања> <нови назив> <систем датотека>             	- преименовање")
	fmt.Println("  обш <путања> <систем датотека>                          	- брисање")
	fmt.Println("  нпфас <путања> <систем датотека>                        	- нова фасцикла")
	fmt.Println("  прист <путања> <овлашћење> <систем датотека>            	- промена приступа")
	fmt.Println("  иб <путања> <ид> <систем датотека>                      	- промена ИБ-а")
	fmt.Println("  вежи <одредиште> <назив везе> <систем датотека>         	- стварање везе")
	fmt.Println("  упдч <путања подизача> <систем датотека>                	- упис подизача")
	fmt.Println("  етк <нова етикета> <систем датотека>                   	- преименовање етикете")
	fmt.Println("  врм <путања> <ознака> <време> <систем датотека>       	- промена времена")
	fmt.Println("    ознака: н (настанак), и (измена), п (приступ)")
	fmt.Println("    време: дд.мм.гггг-чч:мм:сс")
}

func printVerbose(txt string) {
	if cfg.verbose {
		fmt.Printf("датотекомат: %v.\n", txt)
	}
}

func printError(err error) {
	fmt.Printf("датотекомат: %v\n", err)
	os.Exit(1)
}

// logic wrappers

func showFSDetails(fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	readFSDetails(fs)
}

func listTree(fsPath, path string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.ListTree(path); err != nil {
		printError(err)
	}
}

func listEntries(fsPath, path string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.ListEntries(path); err != nil {
		printError(err)
	}
}

func renameEntry(old, new, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.RenameEntry(old, new); err != nil {
		printError(err)
	}
}

func deleteEntry(file, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.DeleteEntry(file); err != nil {
		printError(err)
	}
}

func readFSDetails(fs *sfat.Filesystem) {
	if err := fs.ReadSuperBlock(); err != nil {
		printError(err)
	}

	fs.ShowBootloader()

	if int(fs.SuperBlock.TotalSectors) < 65536 {
		fmt.Println("ТДД:")
		emptyFATentries := 0
		for i := 0; i < int(fs.SuperBlock.TotalSectors); i++ {
			entry, err := fs.FileAllocationTable.GetEntry(fs.Path, fs.SuperBlock, i)
			if err != nil {
				fmt.Printf("  - грешка приликом читања ТДД ставке %v: %v.\n", i, err)
			}

			if entry.DataEntry != 0 {
				fmt.Printf(" - %v: %v\n", i, entry.Details())
			} else {
				emptyFATentries += 1
			}
		}

		if emptyFATentries != 0 {
			fmt.Printf(" - слободних ТДД ставки: %v.\n", emptyFATentries)
		}
	} else {
		fmt.Println("ТДД: (прескачем испис због превеликог броја сектора)")
	}

	emptyFSentries := 0
	fmt.Println("Ставке система датотека:")
	for i := 0; i < int(fs.SuperBlock.TotalFSEntries); i++ {
		entry, err := fs.FSEntries.GetEntry(fs.Path, fs.SuperBlock, i)
		if err != nil {
			printError(fmt.Errorf("не могу прочитати сд ставку: %v", err))
		}

		details := entry.Details()
		if details != "" {
			fmt.Printf(" - %v: %v\n", i, details)
		} else {
			emptyFSentries += 1
		}
	}

	if emptyFSentries != 0 {
		fmt.Printf(" - слободних СД ставки: %v.\n", emptyFSentries)
	}

	fmt.Println(fs.SuperBlock.Details())

	totalSpace := float64(fs.SuperBlock.TotalSectors) * float64(fs.SuperBlock.BytesPerSector)
	availableSpace := float64(fs.SuperBlock.AvailableSectors) * float64(fs.SuperBlock.BytesPerSector)
	fmt.Printf(
		"Укупан простор: %v, слободан простор: %v.\n",
		sfat.HumanReadableUnit(totalSpace),
		sfat.HumanReadableUnit(availableSpace),
	)
}

func copyFileIn(filePath, folderPath, fsPath string) {
	printVerbose(fmt.Sprintf("копирам датотеку \"%v\" у систем датотека \"%v\"", filePath, fsPath))

	timestamp := sfat.TimeToBytes(time.Now())
	if cfg.testTime {
		timestamp = sfat.TestTimestamp
	}
	if err := sfat.CopyFileIn(filePath, folderPath, fsPath, timestamp); err != nil {
		printError(err)
	}
}

func copyFileOut(internalPath, externalPath, fsPath string) {
	printVerbose(fmt.Sprintf("копирам датотеку \"%v\" из система датотека \"%v\"", internalPath, fsPath))

	if err := sfat.CopyFileOut(internalPath, externalPath, fsPath); err != nil {
		printError(err)
	}
}

func writeNewFilesystem(path string) {
	printVerbose(fmt.Sprintf("правим нови систем датотека унутар датотеке %v", path))
	timestamp := sfat.TimeToBytes(time.Now())
	if cfg.testTime {
		timestamp = sfat.TestTimestamp
	}

	fs, err := sfat.New(cfg.totalSectors, cfg.bytesPerSector, cfg.totalFSEntries, cfg.label, path, timestamp)
	if err != nil {
		printError(err)
	}

	printVerbose(fmt.Sprintf("нови систем датотека унутар датотеке %v успешно направљен", fs.Path))
}

func createFolder(folder, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	timestamp := sfat.TimeToBytes(time.Now())
	if cfg.testTime {
		timestamp = sfat.TestTimestamp
	}
	if err := fs.CreateFolder(folder, timestamp); err != nil {
		printError(err)
	}
}

func changeMode(filePath, mode, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.ChangeEntryMode(filePath, mode); err != nil {
		printError(err)
	}
}

func changeUIDGID(filePath, id, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.ChangeUIDGID(filePath, id); err != nil {
		printError(err)
	}
}

func createLink(destPath, linkPath, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	timestamp := sfat.TimeToBytes(time.Now())
	if cfg.testTime {
		timestamp = sfat.TestTimestamp
	}
	if err := fs.CreateLink(destPath, linkPath, timestamp); err != nil {
		printError(err)
	}
}

func writeBootloader(blPath, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.WriteBootArea(blPath); err != nil {
		printError(err)
	}
}

func renameLabel(newLabel, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	if err := fs.RenameLabel(newLabel); err != nil {
		printError(err)
	}
}

func changeTimestamp(filePath, which, timeStr, fsPath string) {
	fs, err := sfat.Read(fsPath)
	if err != nil {
		printError(err)
	}

	newTime, err := sfat.ParseTimestamp(timeStr)
	if err != nil {
		printError(err)
	}

	if err := fs.ChangeTimestamp(filePath, which, newTime); err != nil {
		printError(err)
	}
}
