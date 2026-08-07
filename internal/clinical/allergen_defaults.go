package clinical

type AllergenSeed struct {
	Code, Name, Category string
	Aliases              []string
}

var DefaultAllergens = []AllergenSeed{
	{"penicillins", "Penicillins", "medication", []string{"penicillin", "penicillin antibiotics"}}, {"cephalosporins", "Cephalosporins", "medication", []string{"cephalosporin antibiotics"}},
	{"sulfonamide_antibiotics", "Sulfonamide antibiotics", "medication", []string{"sulfa", "sulphonamide", "sulphur"}}, {"nsaids", "Non-steroidal anti-inflammatory drugs", "medication", []string{"NSAIDs", "anti-inflammatory medicines"}},
	{"amoxicillin", "Amoxicillin", "medication", []string{"amoxycillin"}}, {"amoxicillin_clavulanate", "Amoxicillin/clavulanate", "medication", []string{"co-amoxiclav", "amox-clav", "augmentin"}},
	{"cotrimoxazole", "Co-trimoxazole", "medication", []string{"cotrimoxazole", "trimethoprim-sulfamethoxazole", "septrin"}}, {"aspirin", "Aspirin", "medication", []string{"acetylsalicylic acid"}},
	{"ibuprofen", "Ibuprofen", "medication", nil}, {"diclofenac", "Diclofenac", "medication", nil}, {"paracetamol", "Paracetamol", "medication", []string{"acetaminophen"}},
	{"azithromycin", "Azithromycin", "medication", nil}, {"erythromycin", "Erythromycin", "medication", nil}, {"ciprofloxacin", "Ciprofloxacin", "medication", nil}, {"metronidazole", "Metronidazole", "medication", nil},
	{"nevirapine", "Nevirapine", "medication", nil}, {"abacavir", "Abacavir", "medication", nil}, {"zidovudine", "Zidovudine", "medication", []string{"AZT"}}, {"carbamazepine", "Carbamazepine", "medication", nil}, {"phenytoin", "Phenytoin", "medication", nil},
	{"cows_milk", "Cow's milk", "food", []string{"milk", "dairy"}}, {"egg", "Egg", "food", []string{"eggs"}}, {"peanut", "Peanut", "food", []string{"groundnut"}}, {"tree_nuts", "Tree nuts", "food", []string{"nuts"}},
	{"fish", "Fish", "food", nil}, {"shellfish", "Shellfish", "food", []string{"crustaceans", "molluscs"}}, {"wheat", "Wheat", "food", []string{"gluten"}}, {"soy", "Soy", "food", []string{"soya"}}, {"sesame", "Sesame", "food", nil},
	{"iodinated_contrast", "Iodinated contrast media", "contrast", []string{"iodine contrast", "radiographic contrast"}}, {"gadolinium_contrast", "Gadolinium contrast media", "contrast", []string{"MRI contrast"}},
	{"natural_rubber_latex", "Natural rubber latex", "contact", []string{"latex"}}, {"chlorhexidine", "Chlorhexidine", "contact", nil}, {"bee_venom", "Bee venom", "insect", []string{"bee sting"}}, {"wasp_venom", "Wasp venom", "insect", []string{"wasp sting"}},
	{"house_dust_mite", "House dust mite", "environmental", []string{"dust mite"}}, {"pollen", "Pollen", "environmental", nil}, {"mould", "Mould", "environmental", []string{"mold"}}, {"animal_dander", "Animal dander", "environmental", []string{"pet dander"}},
}
